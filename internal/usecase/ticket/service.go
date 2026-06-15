package ticket

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"ticketing-api/internal/domain"
	"ticketing-api/internal/domain/entity"
	"ticketing-api/internal/domain/repository"
	"ticketing-api/internal/domain/service"
)

const maxNoteLength = 500

// Service implements the Phase 2, 3, 4 and 5 ticket business rules.
type Service struct {
	tickets    repository.TicketRepository
	categories repository.TicketCategoryRepository
	users      repository.UserRepository
	histories  repository.TicketHistoryRepository
	comments   repository.TicketCommentRepository
	tx         service.TxManager
	now        func() time.Time

	// Phase 5 — set via WithPhase5().
	attachments    repository.TicketAttachmentRepository
	storage        service.FileStorage
	maxUploadBytes int64

	// Phase 7 — set via WithPhase7().
	policies      repository.SLAPolicyRepository
	notifications repository.NotificationRepository
}

func NewService(
	tickets repository.TicketRepository,
	categories repository.TicketCategoryRepository,
	users repository.UserRepository,
	histories repository.TicketHistoryRepository,
	comments repository.TicketCommentRepository,
	tx service.TxManager,
) *Service {
	return &Service{
		tickets:    tickets,
		categories: categories,
		users:      users,
		histories:  histories,
		comments:   comments,
		tx:         tx,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

// WithClock overrides the time source. Tests only.
func (s *Service) WithClock(now func() time.Time) *Service {
	cp := *s
	cp.now = now
	return &cp
}

// Actor loads the authenticated user record. Handlers use this so they can
// pass a role-aware actor through to the rest of the API.
func (s *Service) Actor(ctx context.Context, id int64) (*entity.User, error) {
	u, err := s.users.FindByID(ctx, id)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil, domain.ErrUnauthorized
		}
		return nil, err
	}
	return u, nil
}

// ---------- Categories ----------

func (s *Service) ListActiveCategories(ctx context.Context) ([]CategoryOutput, error) {
	cats, err := s.categories.ListActive(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CategoryOutput, 0, len(cats))
	for i := range cats {
		out = append(out, CategoryOutput{
			ID:          cats[i].ID,
			Name:        cats[i].Name,
			Slug:        cats[i].Slug,
			Description: cats[i].Description,
		})
	}
	return out, nil
}

// ---------- Create ----------

// Create writes the ticket and a `created` history row inside one transaction.
func (s *Service) Create(ctx context.Context, actor *entity.User, in CreateTicketInput) (*TicketOutput, error) {
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	if len(in.Title) < 5 || len(in.Title) > 255 {
		return nil, domain.ErrInvalidInput
	}
	if len(in.Description) < 10 {
		return nil, domain.ErrInvalidInput
	}

	priority := strings.TrimSpace(in.Priority)
	if priority == "" {
		priority = entity.TicketPriorityMedium
	}
	if !entity.IsValidTicketPriority(priority) {
		return nil, domain.ErrInvalidInput
	}

	if _, err := s.categories.FindActiveByID(ctx, in.CategoryID); err != nil {
		return nil, err
	}

	t := &entity.Ticket{
		Title:       in.Title,
		Description: in.Description,
		Status:      entity.TicketStatusOpen,
		Priority:    priority,
		CategoryID:  in.CategoryID,
		CreatedBy:   actor.ID,
	}

	err := s.tx.WithinTx(ctx, func(txCtx context.Context) error {
		if err := s.tickets.Create(txCtx, t); err != nil {
			return err
		}

		// Phase 7: load active policy and persist due times. If a policy
		// isn't available (Phase 7 not wired or row missing), the ticket
		// still saves — SLA fields stay null and the DTO omits them.
		if s.policies != nil {
			p, perr := s.policies.FindActiveByPriority(txCtx, t.Priority)
			if perr == nil {
				applyPolicyToTicket(t, p)
				if err := s.tickets.Update(txCtx, t); err != nil {
					return err
				}
			} else if !errors.Is(perr, domain.ErrSLAPolicyNotFound) {
				return perr
			}
		}

		newStatus := entity.TicketStatusOpen
		hist := &entity.TicketHistory{
			TicketID:  t.ID,
			ActorID:   actor.ID,
			Action:    entity.TicketHistoryActionCreated,
			NewStatus: &newStatus,
		}
		return s.histories.Create(txCtx, hist)
	})
	if err != nil {
		return nil, err
	}
	return toOutputAt(t, s.now()), nil
}

// ---------- List ----------

// ScopeCreatedByMe / ScopeAssignedToMe are the canonical scope query values.
const (
	ScopeCreatedByMe  = "created_by_me"
	ScopeAssignedToMe = "assigned_to_me"
)

// defaultPerPage / maxPerPage / maxQueryLength encode the Phase 6 rules.
const (
	defaultPerPage = 20
	maxPerPage     = 100
	maxQueryLength = 200
)

func (s *Service) List(ctx context.Context, actor *entity.User, in ListInput) (*ListOutput, error) {
	page := in.Page
	if page == 0 {
		page = 1
	}
	if page < 1 {
		return nil, domain.ErrInvalidInput
	}
	per := in.PerPage
	if per == 0 {
		per = defaultPerPage
	}
	if per < 1 {
		return nil, domain.ErrInvalidInput
	}
	if per > maxPerPage {
		per = maxPerPage
	}

	if in.Status != "" && !entity.IsValidTicketStatus(in.Status) {
		return nil, domain.ErrInvalidInput
	}
	if in.Priority != "" && !entity.IsValidTicketPriority(in.Priority) {
		return nil, domain.ErrInvalidInput
	}

	// Phase 6 query: trim, cap length.
	query := strings.TrimSpace(in.Query)
	if len(query) > maxQueryLength {
		return nil, domain.ErrInvalidInput
	}

	// Date-range validation: from <= to.
	if in.CreatedFrom != nil && in.CreatedTo != nil && in.CreatedFrom.After(*in.CreatedTo) {
		return nil, domain.ErrInvalidInput
	}

	// Phase 6 sort_by is rejected (422) when unknown; the old behaviour of
	// silently coercing to created_at is gone.
	sortBy, err := strictSortBy(in.SortBy)
	if err != nil {
		return nil, err
	}
	sortOrder, err := strictSortOrder(in.SortOrder)
	if err != nil {
		return nil, err
	}

	scope, err := resolveListScope(actor, in)
	if err != nil {
		return nil, err
	}

	param := repository.TicketListParam{
		Page:        page,
		PerPage:     per,
		Search:      in.Search,
		Query:       query,
		Status:      in.Status,
		Priority:    in.Priority,
		CategoryID:  in.CategoryID,
		CreatedFrom: in.CreatedFrom,
		CreatedTo:   in.CreatedTo,
		SortBy:      sortBy,
		SortOrder:   sortOrder,
		Scope:       scope,
	}
	// Admin-supplied filters only matter once Scope hasn't already pinned them.
	if actor.Role == entity.RoleAdmin {
		param.CreatedBy = in.CreatedBy
		param.AssignedTo = in.AssignedTo
	}

	rows, total, err := s.tickets.List(ctx, param)
	if err != nil {
		return nil, err
	}

	items := make([]TicketOutput, 0, len(rows))
	now := s.now()
	for i := range rows {
		items = append(items, *toOutputAt(&rows[i], now))
	}

	totalPages := 0
	if per > 0 {
		totalPages = int((total + int64(per) - 1) / int64(per))
	}

	return &ListOutput{
		Items:      items,
		Page:       page,
		PerPage:    per,
		Total:      total,
		TotalPages: totalPages,
	}, nil
}

// Phase 6 adds `view=all`, which only admin can use.
const ViewAll = "all"

// resolveListScope encodes list visibility across Phase 3 (`scope`) and
// Phase 6 (`view`):
//
//   - customer: only own tickets, regardless of requested view.
//     `view=assigned_to_me` returns 403; `view=all` returns 422.
//   - agent: created_by_me | assigned_to_me | default(=creator OR assignee).
//     `view=all` is rejected as forbidden.
//   - admin: created_by_me | assigned_to_me | all (default = all).
//     Unknown view values return 422.
//
// When both `View` and `Scope` are set, `View` wins.
func resolveListScope(actor *entity.User, in ListInput) (repository.TicketListScope, error) {
	view := in.View
	if view == "" {
		view = in.Scope
	}
	uid := actor.ID
	switch actor.Role {
	case entity.RoleCustomer:
		switch view {
		case "", ScopeCreatedByMe:
			return repository.TicketListScope{CreatorID: &uid}, nil
		case ScopeAssignedToMe:
			return repository.TicketListScope{}, domain.ErrForbidden
		case ViewAll:
			return repository.TicketListScope{}, domain.ErrInvalidInput
		}
		return repository.TicketListScope{}, domain.ErrInvalidInput
	case entity.RoleAgent:
		switch view {
		case ScopeCreatedByMe:
			return repository.TicketListScope{CreatorID: &uid}, nil
		case ScopeAssignedToMe:
			return repository.TicketListScope{AssigneeID: &uid}, nil
		case "":
			return repository.TicketListScope{CreatorOrAssigneeID: &uid}, nil
		case ViewAll:
			return repository.TicketListScope{}, domain.ErrForbidden
		}
		return repository.TicketListScope{}, domain.ErrInvalidInput
	case entity.RoleAdmin:
		switch view {
		case ScopeCreatedByMe:
			return repository.TicketListScope{CreatorID: &uid}, nil
		case ScopeAssignedToMe:
			return repository.TicketListScope{AssigneeID: &uid}, nil
		case "", ViewAll:
			return repository.TicketListScope{}, nil
		}
		return repository.TicketListScope{}, domain.ErrInvalidInput
	}
	return repository.TicketListScope{}, domain.ErrForbidden
}

// dueSoonMinutes is the "resolution due soon" window used by the dashboard.
// 60 minutes per Phase 7 spec recommendation.
const dueSoonMinutes = 60

// ---------- Dashboard summary ----------

// DashboardSummary returns role-scoped aggregate counts for the actor.
//
// The visibility scope is exactly the same as List with no `view` set —
// customer sees their own, agent sees creator-or-assignee, admin sees all.
func (s *Service) DashboardSummary(ctx context.Context, actor *entity.User) (*DashboardSummaryOutput, error) {
	scope, err := resolveListScope(actor, ListInput{})
	if err != nil {
		return nil, err
	}
	repoSummary, err := s.tickets.Summary(ctx, scope, s.now(), dueSoonMinutes)
	if err != nil {
		return nil, err
	}
	out := &DashboardSummaryOutput{
		TotalTickets: repoSummary.Total,
		ByStatus:     repoSummary.ByStatus,
		ByPriority:   repoSummary.ByPriority,
		ByCategory:   make([]DashboardCategoryCount, 0, len(repoSummary.ByCategory)),
		SLA: DashboardSLABlock{
			ResponseBreached:   repoSummary.SLAResponseBreached,
			ResolutionBreached: repoSummary.SLAResolutionBreached,
			ResolutionDueSoon:  repoSummary.SLAResolutionDueSoon,
		},
	}
	if s.notifications != nil {
		if count, uerr := s.notifications.UnreadCount(ctx, actor.ID); uerr == nil {
			out.UnreadNotifications = count
		}
	}
	for _, c := range repoSummary.ByCategory {
		out.ByCategory = append(out.ByCategory, DashboardCategoryCount{
			CategoryID:   c.CategoryID,
			CategoryName: c.CategoryName,
			Total:        c.Total,
		})
	}
	return out, nil
}

// ---------- Detail ----------

func (s *Service) Get(ctx context.Context, actor *entity.User, id uuid.UUID) (*TicketOutput, error) {
	t, err := s.tickets.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canViewTicket(actor, t) {
		return nil, domain.ErrTicketNotFound
	}
	return toOutputAt(t, s.now()), nil
}

// ---------- Update fields ----------

func (s *Service) Update(ctx context.Context, actor *entity.User, id uuid.UUID, in UpdateTicketInput) (*TicketOutput, error) {
	if in.Title == nil && in.Description == nil && in.CategoryID == nil && in.Priority == nil {
		return nil, domain.ErrInvalidInput
	}

	t, err := s.tickets.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := authorizeUpdate(actor, t); err != nil {
		return nil, err
	}
	// Phase 5: customers cannot change classification post-creation. The
	// dedicated /classification endpoint is the only way to change those.
	if actor.Role == entity.RoleCustomer && (in.CategoryID != nil || in.Priority != nil) {
		return nil, domain.ErrForbidden
	}

	if in.Title != nil {
		title := strings.TrimSpace(*in.Title)
		if len(title) < 5 || len(title) > 255 {
			return nil, domain.ErrInvalidInput
		}
		t.Title = title
	}
	if in.Description != nil {
		desc := strings.TrimSpace(*in.Description)
		if len(desc) < 10 {
			return nil, domain.ErrInvalidInput
		}
		t.Description = desc
	}
	if in.Priority != nil {
		p := strings.TrimSpace(*in.Priority)
		if !entity.IsValidTicketPriority(p) {
			return nil, domain.ErrInvalidInput
		}
		t.Priority = p
	}
	if in.CategoryID != nil {
		cat, err := s.categories.FindActiveByID(ctx, *in.CategoryID)
		if err != nil {
			return nil, err
		}
		t.CategoryID = cat.ID
	}

	if err := s.tickets.Update(ctx, t); err != nil {
		return nil, err
	}
	return toOutputAt(t, s.now()), nil
}

// ---------- Update status (transactional) ----------

func (s *Service) UpdateStatus(ctx context.Context, actor *entity.User, id uuid.UUID, in UpdateStatusInput) (*TicketOutput, error) {
	next := strings.TrimSpace(in.Status)
	if !entity.IsValidTicketStatus(next) {
		return nil, domain.ErrInvalidInput
	}
	note, err := normaliseNote(in.Note)
	if err != nil {
		return nil, err
	}

	var out *TicketOutput
	err = s.tx.WithinTx(ctx, func(txCtx context.Context) error {
		t, err := s.tickets.FindByIDForUpdate(txCtx, id)
		if err != nil {
			return err
		}
		if !canViewTicket(actor, t) {
			return domain.ErrTicketNotFound
		}
		if err := authorizeStatusChange(actor, t, next); err != nil {
			return err
		}
		if !entity.AllowedTicketTransition(t.Status, next) {
			return domain.ErrInvalidStatusTransition
		}
		// Phase 3 rule 4: an unassigned ticket cannot move open -> in_progress
		// until an admin has assigned it.
		if next == entity.TicketStatusInProgress && t.Status == entity.TicketStatusOpen && t.AssignedTo == nil {
			return domain.ErrTicketNotAssigned
		}

		prev := t.Status
		t.Status = next
		now := s.now()

		// Phase 7 SLA timestamps move with the status:
		//   in_progress: set first_responded_at when null
		//   resolved:    set resolved_at
		//   closed:      set closed_at
		//   reopened:    clear resolved_at and closed_at
		// Agents/admins moving the ticket count as a first response.
		switch next {
		case entity.TicketStatusInProgress:
			if t.FirstRespondedAt == nil && (actor.Role == entity.RoleAdmin || actor.Role == entity.RoleAgent) {
				cp := now
				t.FirstRespondedAt = &cp
			}
		case entity.TicketStatusResolved:
			cp := now
			t.ResolvedAt = &cp
		case entity.TicketStatusClosed:
			cp := now
			t.ClosedAt = &cp
		case entity.TicketStatusReopened:
			t.ResolvedAt = nil
			t.ClosedAt = nil
		}

		if err := s.tickets.UpdateStatus(txCtx, t); err != nil {
			return err
		}

		prevPtr := prev
		nextPtr := next
		hist := &entity.TicketHistory{
			TicketID:  t.ID,
			ActorID:   actor.ID,
			Action:    entity.TicketHistoryActionStatusChanged,
			OldStatus: &prevPtr,
			NewStatus: &nextPtr,
			Note:      note,
		}
		if err := s.histories.Create(txCtx, hist); err != nil {
			return err
		}

		// Phase 7: notify creator if the actor isn't the creator.
		if t.CreatedBy != actor.ID {
			n := notificationForStatusChange(actor, t, t.CreatedBy, prev, next)
			if err := s.notify(txCtx, []entity.Notification{n}); err != nil {
				return err
			}
		}

		// Refresh with preloads inside the same tx so we return the up-to-date
		// snapshot to the caller.
		reloaded, err := s.tickets.FindByID(txCtx, id)
		if err != nil {
			return err
		}
		out = toOutputAt(reloaded, now)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ---------- Assignment (transactional, admin only) ----------

func (s *Service) Assign(ctx context.Context, actor *entity.User, id uuid.UUID, in AssignInput) (*TicketOutput, error) {
	if actor.Role != entity.RoleAdmin {
		return nil, domain.ErrForbidden
	}
	note, err := normaliseNote(in.Note)
	if err != nil {
		return nil, err
	}

	var out *TicketOutput
	err = s.tx.WithinTx(ctx, func(txCtx context.Context) error {
		t, err := s.tickets.FindByIDForUpdate(txCtx, id)
		if err != nil {
			return err
		}

		oldAssigneeID := t.AssignedTo
		var newAssigneeID *int64
		action := entity.TicketHistoryActionAssigned

		if in.AgentID == nil {
			if oldAssigneeID == nil {
				return domain.ErrInvalidInput
			}
			action = entity.TicketHistoryActionUnassigned
			t.AssignedTo = nil
			t.AssignedAt = nil
		} else {
			agent, err := s.users.FindByIDAndRole(txCtx, *in.AgentID, entity.RoleAgent)
			if err != nil {
				if errors.Is(err, domain.ErrUserNotFound) {
					return domain.ErrInvalidAssignee
				}
				return err
			}
			now := s.now()
			t.AssignedTo = &agent.ID
			t.AssignedAt = &now
			newAssigneeID = &agent.ID
			if oldAssigneeID != nil && *oldAssigneeID != agent.ID {
				action = entity.TicketHistoryActionReassigned
			} else if oldAssigneeID != nil && *oldAssigneeID == agent.ID {
				return domain.ErrInvalidInput
			}
		}

		if err := s.tickets.UpdateAssignment(txCtx, t); err != nil {
			return err
		}
		hist := &entity.TicketHistory{
			TicketID:      t.ID,
			ActorID:       actor.ID,
			Action:        action,
			OldAssigneeID: oldAssigneeID,
			NewAssigneeID: newAssigneeID,
			Note:          note,
		}
		if err := s.histories.Create(txCtx, hist); err != nil {
			return err
		}

		// Phase 7: notify the new assignee unless they're the actor.
		if newAssigneeID != nil && *newAssigneeID != actor.ID {
			notificationType := entity.NotificationTypeTicketAssigned
			if action == entity.TicketHistoryActionReassigned {
				notificationType = entity.NotificationTypeTicketReassigned
			}
			n := notificationForAssignment(actor, t, *newAssigneeID, notificationType)
			if err := s.notify(txCtx, []entity.Notification{n}); err != nil {
				return err
			}
		}

		reloaded, err := s.tickets.FindByID(txCtx, id)
		if err != nil {
			return err
		}
		out = toOutputAt(reloaded, s.now())
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ---------- Histories ----------

func (s *Service) ListHistories(ctx context.Context, actor *entity.User, ticketID uuid.UUID) ([]HistoryOutput, error) {
	t, err := s.tickets.FindByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if !canViewTicket(actor, t) {
		return nil, domain.ErrTicketNotFound
	}
	rows, err := s.histories.ListByTicketID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	out := make([]HistoryOutput, 0, len(rows))
	for i := range rows {
		out = append(out, toHistoryOutput(&rows[i]))
	}
	return out, nil
}

// ---------- Soft delete ----------

func (s *Service) Delete(ctx context.Context, actor *entity.User, id uuid.UUID) error {
	t, err := s.tickets.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !canViewTicket(actor, t) {
		return domain.ErrTicketNotFound
	}
	switch actor.Role {
	case entity.RoleAdmin:
		// always allowed
	case entity.RoleCustomer:
		if t.CreatedBy != actor.ID || t.Status != entity.TicketStatusOpen {
			return domain.ErrForbidden
		}
	case entity.RoleAgent:
		return domain.ErrForbidden
	default:
		return domain.ErrForbidden
	}
	return s.tickets.SoftDelete(ctx, id)
}

// ---------- Helpers ----------

func canViewTicket(actor *entity.User, t *entity.Ticket) bool {
	if t == nil {
		return false
	}
	switch actor.Role {
	case entity.RoleAdmin:
		return true
	case entity.RoleAgent:
		if t.CreatedBy == actor.ID {
			return true
		}
		return t.AssignedTo != nil && *t.AssignedTo == actor.ID
	case entity.RoleCustomer:
		return t.CreatedBy == actor.ID
	}
	return false
}

func authorizeUpdate(actor *entity.User, t *entity.Ticket) error {
	switch actor.Role {
	case entity.RoleAdmin:
		return nil
	case entity.RoleCustomer:
		if t.CreatedBy != actor.ID {
			return domain.ErrTicketNotFound
		}
		if t.Status != entity.TicketStatusOpen {
			return domain.ErrForbidden
		}
		return nil
	case entity.RoleAgent:
		if t.AssignedTo == nil || *t.AssignedTo != actor.ID {
			return domain.ErrForbidden
		}
		return nil
	}
	return domain.ErrForbidden
}

// authorizeStatusChange encodes the Phase 3 actor rules.
//
//	admin              -> any allowed transition
//	agent assigned     -> any allowed transition
//	customer creator   -> only closed -> reopened
//	everyone else      -> forbidden / not found
func authorizeStatusChange(actor *entity.User, t *entity.Ticket, next string) error {
	switch actor.Role {
	case entity.RoleAdmin:
		return nil
	case entity.RoleAgent:
		if t.AssignedTo == nil || *t.AssignedTo != actor.ID {
			return domain.ErrForbidden
		}
		return nil
	case entity.RoleCustomer:
		if t.CreatedBy != actor.ID {
			return domain.ErrTicketNotFound
		}
		if t.Status == entity.TicketStatusClosed && next == entity.TicketStatusReopened {
			return nil
		}
		return domain.ErrForbidden
	}
	return domain.ErrForbidden
}

// userSummary builds a UserSummary that includes role when known. Never
// returns the password hash or any other sensitive field.
func userSummary(u *entity.User) *UserSummary {
	return &UserSummary{ID: u.ID, Name: u.Name, Role: u.Role}
}

// ---------- Phase 4: comments + timeline ----------

const (
	minCommentLength = 1
	maxCommentLength = 5000
)

// AddComment creates a comment on a ticket the actor can access.
func (s *Service) AddComment(ctx context.Context, actor *entity.User, ticketID uuid.UUID, in AddCommentInput) (*CommentOutput, error) {
	body, err := validateCommentBody(in.Body)
	if err != nil {
		return nil, err
	}

	var out *CommentOutput
	err = s.tx.WithinTx(ctx, func(txCtx context.Context) error {
		t, err := s.tickets.FindByIDForUpdate(txCtx, ticketID)
		if err != nil {
			return err
		}
		if !canViewTicket(actor, t) {
			return domain.ErrTicketNotFound
		}

		// Phase 7: agent/admin first comment counts as the first response.
		if (actor.Role == entity.RoleAdmin || actor.Role == entity.RoleAgent) && t.FirstRespondedAt == nil {
			now := s.now()
			t.FirstRespondedAt = &now
			if err := s.tickets.UpdateStatus(txCtx, t); err != nil {
				return err
			}
		}

		c := &entity.TicketComment{TicketID: ticketID, AuthorID: actor.ID, Body: body}
		if err := s.comments.Create(txCtx, c); err != nil {
			return err
		}

		// Phase 7 notifications:
		//   - creator commenting: notify assignee (if any).
		//   - agent/admin commenting: notify creator (if not the actor).
		var notes []entity.Notification
		if actor.ID == t.CreatedBy {
			if t.AssignedTo != nil && *t.AssignedTo != actor.ID {
				notes = append(notes, notificationForComment(actor, t, *t.AssignedTo))
			}
		} else if t.CreatedBy != actor.ID {
			notes = append(notes, notificationForComment(actor, t, t.CreatedBy))
		}
		if err := s.notify(txCtx, notes); err != nil {
			return err
		}

		out = toCommentOutput(c)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListComments returns all comments for an accessible ticket, oldest first.
func (s *Service) ListComments(ctx context.Context, actor *entity.User, ticketID uuid.UUID) ([]CommentOutput, error) {
	t, err := s.tickets.FindByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if !canViewTicket(actor, t) {
		return nil, domain.ErrTicketNotFound
	}
	rows, err := s.comments.ListByTicketID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	out := make([]CommentOutput, 0, len(rows))
	for i := range rows {
		out = append(out, *toCommentOutput(&rows[i]))
	}
	return out, nil
}

// UpdateComment edits the body. Author or admin only. The comment must
// belong to the path ticket.
func (s *Service) UpdateComment(ctx context.Context, actor *entity.User, ticketID, commentID uuid.UUID, in UpdateCommentInput) (*CommentOutput, error) {
	body, err := validateCommentBody(in.Body)
	if err != nil {
		return nil, err
	}
	t, err := s.tickets.FindByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if !canViewTicket(actor, t) {
		return nil, domain.ErrTicketNotFound
	}
	c, err := s.comments.FindByID(ctx, commentID)
	if err != nil {
		return nil, err
	}
	if c.TicketID != ticketID {
		return nil, domain.ErrCommentNotFound
	}
	if !canEditOrDeleteComment(actor, c) {
		return nil, domain.ErrForbidden
	}
	c.Body = body
	if err := s.comments.Update(ctx, c); err != nil {
		return nil, err
	}
	return toCommentOutput(c), nil
}

// DeleteComment hard-deletes a comment owned by the actor or by an admin.
func (s *Service) DeleteComment(ctx context.Context, actor *entity.User, ticketID, commentID uuid.UUID) error {
	t, err := s.tickets.FindByID(ctx, ticketID)
	if err != nil {
		return err
	}
	if !canViewTicket(actor, t) {
		return domain.ErrTicketNotFound
	}
	c, err := s.comments.FindByID(ctx, commentID)
	if err != nil {
		return err
	}
	if c.TicketID != ticketID {
		return domain.ErrCommentNotFound
	}
	if !canEditOrDeleteComment(actor, c) {
		return domain.ErrForbidden
	}
	return s.comments.Delete(ctx, commentID)
}

// GetTimeline returns history rows + comments interleaved chronologically.
// Sort key: occurred_at ASC, then ID ASC (stable tie-break).
func (s *Service) GetTimeline(ctx context.Context, actor *entity.User, ticketID uuid.UUID) ([]TimelineItemOutput, error) {
	t, err := s.tickets.FindByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if !canViewTicket(actor, t) {
		return nil, domain.ErrTicketNotFound
	}
	histories, err := s.histories.ListByTicketID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	comments, err := s.comments.ListByTicketID(ctx, ticketID)
	if err != nil {
		return nil, err
	}

	items := make([]TimelineItemOutput, 0, len(histories)+len(comments))
	for i := range histories {
		h := toHistoryOutput(&histories[i])
		items = append(items, TimelineItemOutput{
			Type:       "history",
			OccurredAt: histories[i].CreatedAt,
			History:    &h,
		})
	}
	for i := range comments {
		c := toCommentOutput(&comments[i])
		items = append(items, TimelineItemOutput{
			Type:       "comment",
			OccurredAt: comments[i].CreatedAt,
			Comment:    c,
		})
	}
	sort.SliceStable(items, func(a, b int) bool {
		if items[a].OccurredAt.Equal(items[b].OccurredAt) {
			return timelineItemID(items[a]).String() < timelineItemID(items[b]).String()
		}
		return items[a].OccurredAt.Before(items[b].OccurredAt)
	})
	return items, nil
}

func timelineItemID(it TimelineItemOutput) uuid.UUID {
	if it.History != nil {
		return it.History.ID
	}
	if it.Comment != nil {
		return it.Comment.ID
	}
	return uuid.Nil
}

func validateCommentBody(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) < minCommentLength || len(trimmed) > maxCommentLength {
		return "", domain.ErrInvalidInput
	}
	return trimmed, nil
}

// canEditOrDeleteComment: author or admin.
func canEditOrDeleteComment(actor *entity.User, c *entity.TicketComment) bool {
	if actor.Role == entity.RoleAdmin {
		return true
	}
	return c.AuthorID == actor.ID
}

func toCommentOutput(c *entity.TicketComment) *CommentOutput {
	out := &CommentOutput{
		ID:        c.ID,
		TicketID:  c.TicketID,
		Body:      c.Body,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
	if c.Author != nil {
		out.Author = userSummary(c.Author)
	} else {
		out.Author = &UserSummary{ID: c.AuthorID}
	}
	return out
}

// ---------- helpers (kept at bottom) ----------

// Phase 6 allow-list. status is added; code stays for legacy callers.
var allowedSortBy = map[string]struct{}{
	"created_at": {},
	"updated_at": {},
	"priority":   {},
	"status":     {},
	"code":       {},
}

// strictSortBy returns the default when blank but rejects unknown values
// with domain.ErrInvalidInput. Phase 6 mandates a hard 422 rather than
// silent coercion.
func strictSortBy(in string) (string, error) {
	in = strings.TrimSpace(in)
	if in == "" {
		return "created_at", nil
	}
	if _, ok := allowedSortBy[in]; !ok {
		return "", domain.ErrInvalidInput
	}
	return in, nil
}

func strictSortOrder(in string) (string, error) {
	in = strings.ToLower(strings.TrimSpace(in))
	switch in {
	case "":
		return "desc", nil
	case "asc", "desc":
		return in, nil
	}
	return "", domain.ErrInvalidInput
}

func normaliseNote(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil, nil
	}
	if len(trimmed) > maxNoteLength {
		return nil, domain.ErrInvalidInput
	}
	return &trimmed, nil
}

func toOutput(t *entity.Ticket) *TicketOutput {
	return toOutputAt(t, time.Now().UTC())
}

// toOutputAt is the clock-aware variant used by Phase 7 SLA derivations.
// Use sites should call s.now() to honour the Service's injected clock.
func toOutputAt(t *entity.Ticket, now time.Time) *TicketOutput {
	o := &TicketOutput{
		ID:          t.ID,
		Code:        t.Code,
		Title:       t.Title,
		Description: t.Description,
		Status:      t.Status,
		Priority:    t.Priority,
		AssignedAt:  t.AssignedAt,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
		SLA:         computeSLAOutput(t, now),
	}
	if t.Category != nil {
		o.Category = &CategorySummary{
			ID:   t.Category.ID,
			Name: t.Category.Name,
			Slug: t.Category.Slug,
		}
	} else {
		o.Category = &CategorySummary{ID: t.CategoryID}
	}
	if t.Creator != nil {
		o.Creator = userSummary(t.Creator)
	} else {
		o.Creator = &UserSummary{ID: t.CreatedBy}
	}
	if t.Assignee != nil {
		o.Assignee = userSummary(t.Assignee)
	} else if t.AssignedTo != nil {
		o.Assignee = &UserSummary{ID: *t.AssignedTo}
	}
	return o
}

func toHistoryOutput(h *entity.TicketHistory) HistoryOutput {
	out := HistoryOutput{
		ID:        h.ID,
		TicketID:  h.TicketID,
		Action:    h.Action,
		OldStatus: h.OldStatus,
		NewStatus: h.NewStatus,
		Note:      h.Note,
		CreatedAt: h.CreatedAt,
	}
	if h.Actor != nil {
		out.Actor = userSummary(h.Actor)
	} else {
		out.Actor = &UserSummary{ID: h.ActorID}
	}
	if h.OldAssignee != nil {
		out.OldAssignee = userSummary(h.OldAssignee)
	} else if h.OldAssigneeID != nil {
		out.OldAssignee = &UserSummary{ID: *h.OldAssigneeID}
	}
	if h.NewAssignee != nil {
		out.NewAssignee = userSummary(h.NewAssignee)
	} else if h.NewAssigneeID != nil {
		out.NewAssignee = &UserSummary{ID: *h.NewAssigneeID}
	}
	return out
}
