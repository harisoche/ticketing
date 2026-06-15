package ticket

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"ticketing-api/internal/domain"
	"ticketing-api/internal/domain/entity"
	"ticketing-api/internal/domain/repository"
)

// =========== fakes ===========

type fakeTicketRepo struct {
	mu     sync.Mutex
	byID   map[uuid.UUID]*entity.Ticket
	seq    int
	failOn string // "" | method name
	clock  func() time.Time
}

func newFakeTicketRepo() *fakeTicketRepo {
	return &fakeTicketRepo{
		byID:  map[uuid.UUID]*entity.Ticket{},
		clock: func() time.Time { return time.Now().UTC() },
	}
}

func (r *fakeTicketRepo) now() time.Time { return r.clock() }

func (r *fakeTicketRepo) Create(_ context.Context, t *entity.Ticket) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failOn == "Create" {
		return errors.New("induced failure")
	}
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	r.seq++
	t.Code = fmt.Sprintf("TKT-%08d", r.seq)
	t.CreatedAt = r.now()
	t.UpdatedAt = t.CreatedAt
	clone := *t
	r.byID[t.ID] = &clone
	return nil
}

func (r *fakeTicketRepo) FindByID(_ context.Context, id uuid.UUID) (*entity.Ticket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.byID[id]
	if !ok || t.DeletedAt != nil {
		return nil, domain.ErrTicketNotFound
	}
	clone := *t
	return &clone, nil
}

func (r *fakeTicketRepo) FindByIDForUpdate(ctx context.Context, id uuid.UUID) (*entity.Ticket, error) {
	return r.FindByID(ctx, id)
}

func (r *fakeTicketRepo) List(_ context.Context, p repository.TicketListParam) ([]entity.Ticket, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	matches := []entity.Ticket{}
	for _, t := range r.byID {
		if t.DeletedAt != nil {
			continue
		}
		if p.CreatedBy != nil && t.CreatedBy != *p.CreatedBy {
			continue
		}
		if p.AssignedTo != nil {
			if t.AssignedTo == nil || *t.AssignedTo != *p.AssignedTo {
				continue
			}
		}
		if p.Scope.CreatorID != nil && t.CreatedBy != *p.Scope.CreatorID {
			continue
		}
		if p.Scope.AssigneeID != nil {
			if t.AssignedTo == nil || *t.AssignedTo != *p.Scope.AssigneeID {
				continue
			}
		}
		if p.Scope.CreatorOrAssigneeID != nil {
			uid := *p.Scope.CreatorOrAssigneeID
			if t.CreatedBy != uid && (t.AssignedTo == nil || *t.AssignedTo != uid) {
				continue
			}
		}
		if p.Status != "" && t.Status != p.Status {
			continue
		}
		matches = append(matches, *t)
	}
	return matches, int64(len(matches)), nil
}

func (r *fakeTicketRepo) Update(_ context.Context, t *entity.Ticket) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.byID[t.ID]
	if !ok || cur.DeletedAt != nil {
		return domain.ErrTicketNotFound
	}
	t.CreatedAt = cur.CreatedAt
	t.Code = cur.Code
	t.UpdatedAt = time.Now().UTC()
	clone := *t
	r.byID[t.ID] = &clone
	return nil
}

func (r *fakeTicketRepo) UpdateAssignment(_ context.Context, t *entity.Ticket) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.byID[t.ID]
	if !ok {
		return domain.ErrTicketNotFound
	}
	cur.AssignedTo = t.AssignedTo
	cur.AssignedAt = t.AssignedAt
	cur.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *fakeTicketRepo) UpdateStatus(_ context.Context, t *entity.Ticket) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.byID[t.ID]
	if !ok {
		return domain.ErrTicketNotFound
	}
	cur.Status = t.Status
	cur.FirstRespondedAt = t.FirstRespondedAt
	cur.ResolvedAt = t.ResolvedAt
	cur.ClosedAt = t.ClosedAt
	cur.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *fakeTicketRepo) Summary(_ context.Context, scope repository.TicketListScope, now time.Time, dueSoonMinutes int) (*repository.TicketSummary, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	summary := &repository.TicketSummary{
		ByStatus: map[string]int64{
			"open": 0, "in_progress": 0, "resolved": 0, "closed": 0, "reopened": 0,
		},
		ByPriority: map[string]int64{"low": 0, "medium": 0, "high": 0, "urgent": 0},
	}
	catByID := map[uuid.UUID]string{}
	catTotals := map[uuid.UUID]int64{}
	for _, t := range r.byID {
		if t.DeletedAt != nil {
			continue
		}
		if scope.CreatorID != nil && t.CreatedBy != *scope.CreatorID {
			continue
		}
		if scope.AssigneeID != nil {
			if t.AssignedTo == nil || *t.AssignedTo != *scope.AssigneeID {
				continue
			}
		}
		if scope.CreatorOrAssigneeID != nil {
			uid := *scope.CreatorOrAssigneeID
			if t.CreatedBy != uid && (t.AssignedTo == nil || *t.AssignedTo != uid) {
				continue
			}
		}
		summary.Total++
		summary.ByStatus[t.Status]++
		summary.ByPriority[t.Priority]++
		if t.Category != nil {
			catByID[t.Category.ID] = t.Category.Name
		} else {
			catByID[t.CategoryID] = ""
		}
		catTotals[t.CategoryID]++

		// Phase 7 SLA aggregates.
		if t.FirstRespondedAt == nil && t.ResponseDueAt != nil && now.After(*t.ResponseDueAt) {
			summary.SLAResponseBreached++
		}
		if t.ResolvedAt == nil && t.ResolutionDueAt != nil {
			soon := now.Add(time.Duration(dueSoonMinutes) * time.Minute)
			if now.After(*t.ResolutionDueAt) {
				summary.SLAResolutionBreached++
			} else if !t.ResolutionDueAt.Before(now) && !t.ResolutionDueAt.After(soon) {
				summary.SLAResolutionDueSoon++
			}
		}
	}
	for id, total := range catTotals {
		summary.ByCategory = append(summary.ByCategory, repository.TicketSummaryCategoryCount{
			CategoryID:   id,
			CategoryName: catByID[id],
			Total:        total,
		})
	}
	// Sort by total DESC then name ASC for determinism.
	for i := range summary.ByCategory {
		for j := i + 1; j < len(summary.ByCategory); j++ {
			a, b := summary.ByCategory[i], summary.ByCategory[j]
			if b.Total > a.Total || (b.Total == a.Total && b.CategoryName < a.CategoryName) {
				summary.ByCategory[i], summary.ByCategory[j] = summary.ByCategory[j], summary.ByCategory[i]
			}
		}
	}
	return summary, nil
}

func (r *fakeTicketRepo) SoftDelete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.byID[id]
	if !ok || cur.DeletedAt != nil {
		return domain.ErrTicketNotFound
	}
	now := time.Now().UTC()
	cur.DeletedAt = &now
	return nil
}

type fakeCategoryRepo struct {
	mu       sync.Mutex
	byID     map[uuid.UUID]*entity.TicketCategory
	inactive map[uuid.UUID]bool
}

func newFakeCategoryRepo() *fakeCategoryRepo {
	return &fakeCategoryRepo{byID: map[uuid.UUID]*entity.TicketCategory{}, inactive: map[uuid.UUID]bool{}}
}

func (r *fakeCategoryRepo) addActive(name string) *entity.TicketCategory {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := &entity.TicketCategory{ID: uuid.New(), Name: name, Slug: name, IsActive: true}
	r.byID[c.ID] = c
	return c
}

func (r *fakeCategoryRepo) ListActive(_ context.Context) ([]entity.TicketCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []entity.TicketCategory{}
	for id, c := range r.byID {
		if r.inactive[id] {
			continue
		}
		out = append(out, *c)
	}
	return out, nil
}

func (r *fakeCategoryRepo) FindActiveByID(_ context.Context, id uuid.UUID) (*entity.TicketCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.byID[id]
	if !ok || r.inactive[id] {
		return nil, domain.ErrTicketCategoryNotFound
	}
	clone := *c
	return &clone, nil
}

func (r *fakeCategoryRepo) Create(_ context.Context, c *entity.TicketCategory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.byID {
		if existing.Name == c.Name || existing.Slug == c.Slug {
			return domain.ErrCategoryConflict
		}
	}
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	c.CreatedAt = time.Now().UTC()
	c.UpdatedAt = c.CreatedAt
	clone := *c
	r.byID[c.ID] = &clone
	if !c.IsActive {
		r.inactive[c.ID] = true
	}
	return nil
}

func (r *fakeCategoryRepo) List(_ context.Context, includeInactive bool) ([]entity.TicketCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []entity.TicketCategory{}
	for id, c := range r.byID {
		if !includeInactive && r.inactive[id] {
			continue
		}
		out = append(out, *c)
	}
	return out, nil
}

func (r *fakeCategoryRepo) FindByID(_ context.Context, id uuid.UUID) (*entity.TicketCategory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.byID[id]
	if !ok {
		return nil, domain.ErrTicketCategoryNotFound
	}
	clone := *c
	return &clone, nil
}

func (r *fakeCategoryRepo) Update(_ context.Context, c *entity.TicketCategory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.byID[c.ID]
	if !ok {
		return domain.ErrTicketCategoryNotFound
	}
	for id, existing := range r.byID {
		if id == c.ID {
			continue
		}
		if existing.Name == c.Name || existing.Slug == c.Slug {
			return domain.ErrCategoryConflict
		}
	}
	cur.Name = c.Name
	cur.Slug = c.Slug
	cur.Description = c.Description
	cur.IsActive = c.IsActive
	cur.UpdatedAt = time.Now().UTC()
	if c.IsActive {
		delete(r.inactive, c.ID)
	} else {
		r.inactive[c.ID] = true
	}
	*c = *cur
	return nil
}

type fakeUserRepo struct {
	mu     sync.Mutex
	byID   map[int64]*entity.User
	nextID int64
}

func newFakeUserRepo() *fakeUserRepo {
	return &fakeUserRepo{byID: map[int64]*entity.User{}}
}

func (r *fakeUserRepo) add(role, name string) *entity.User {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	u := &entity.User{ID: r.nextID, Name: name, Role: role}
	r.byID[u.ID] = u
	return u
}

func (r *fakeUserRepo) Create(_ context.Context, _ *entity.User) error { return nil }
func (r *fakeUserRepo) FindByEmail(_ context.Context, _ string) (*entity.User, error) {
	return nil, domain.ErrUserNotFound
}
func (r *fakeUserRepo) UpdateName(_ context.Context, _ int64, _ string) (*entity.User, error) {
	return nil, nil
}

func (r *fakeUserRepo) FindByID(_ context.Context, id int64) (*entity.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	clone := *u
	return &clone, nil
}

func (r *fakeUserRepo) FindByIDAndRole(_ context.Context, id int64, role string) (*entity.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[id]
	if !ok || u.Role != role {
		return nil, domain.ErrUserNotFound
	}
	clone := *u
	return &clone, nil
}

type fakeHistoryRepo struct {
	mu       sync.Mutex
	byTicket map[uuid.UUID][]entity.TicketHistory
	failOn   string // "" | "Create"
}

func newFakeHistoryRepo() *fakeHistoryRepo {
	return &fakeHistoryRepo{byTicket: map[uuid.UUID][]entity.TicketHistory{}}
}

func (r *fakeHistoryRepo) Create(_ context.Context, h *entity.TicketHistory) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failOn == "Create" {
		return errors.New("induced history failure")
	}
	if h.ID == uuid.Nil {
		h.ID = uuid.New()
	}
	h.CreatedAt = time.Now().UTC()
	r.byTicket[h.TicketID] = append(r.byTicket[h.TicketID], *h)
	return nil
}

func (r *fakeHistoryRepo) ListByTicketID(_ context.Context, ticketID uuid.UUID) ([]entity.TicketHistory, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := append([]entity.TicketHistory(nil), r.byTicket[ticketID]...)
	return rows, nil
}

type fakeCommentRepo struct {
	mu       sync.Mutex
	byID     map[uuid.UUID]*entity.TicketComment
	byTicket map[uuid.UUID][]uuid.UUID
	users    *fakeUserRepo // for author preload
	seq      int64
}

func newFakeCommentRepo(users *fakeUserRepo) *fakeCommentRepo {
	return &fakeCommentRepo{
		byID:     map[uuid.UUID]*entity.TicketComment{},
		byTicket: map[uuid.UUID][]uuid.UUID{},
		users:    users,
	}
}

func (r *fakeCommentRepo) preload(c *entity.TicketComment) {
	if r.users == nil {
		return
	}
	if u, ok := r.users.byID[c.AuthorID]; ok {
		clone := *u
		c.Author = &clone
	}
}

func (r *fakeCommentRepo) Create(_ context.Context, c *entity.TicketComment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	r.seq++
	c.CreatedAt = time.Now().UTC().Add(time.Duration(r.seq) * time.Millisecond)
	c.UpdatedAt = c.CreatedAt
	r.preload(c)
	clone := *c
	r.byID[c.ID] = &clone
	r.byTicket[c.TicketID] = append(r.byTicket[c.TicketID], c.ID)
	return nil
}

func (r *fakeCommentRepo) ListByTicketID(_ context.Context, ticketID uuid.UUID) ([]entity.TicketComment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := r.byTicket[ticketID]
	out := make([]entity.TicketComment, 0, len(ids))
	for _, id := range ids {
		if c, ok := r.byID[id]; ok {
			clone := *c
			r.preload(&clone)
			out = append(out, clone)
		}
	}
	return out, nil
}

func (r *fakeCommentRepo) FindByID(_ context.Context, id uuid.UUID) (*entity.TicketComment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.byID[id]
	if !ok {
		return nil, domain.ErrCommentNotFound
	}
	clone := *c
	r.preload(&clone)
	return &clone, nil
}

func (r *fakeCommentRepo) Update(_ context.Context, c *entity.TicketComment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.byID[c.ID]
	if !ok {
		return domain.ErrCommentNotFound
	}
	cur.Body = c.Body
	cur.UpdatedAt = time.Now().UTC()
	clone := *cur
	r.preload(&clone)
	*c = clone
	return nil
}

func (r *fakeCommentRepo) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.byID[id]
	if !ok {
		return domain.ErrCommentNotFound
	}
	delete(r.byID, id)
	tids := r.byTicket[c.TicketID]
	for i, x := range tids {
		if x == id {
			r.byTicket[c.TicketID] = append(tids[:i], tids[i+1:]...)
			break
		}
	}
	return nil
}

// fakeTxManager runs fn directly, but also tracks state so we can verify
// rollback semantics in tests.
type fakeTxManager struct {
	tickets  *fakeTicketRepo
	hist     *fakeHistoryRepo
	rollback bool
}

func newFakeTxManager(t *fakeTicketRepo, h *fakeHistoryRepo) *fakeTxManager {
	return &fakeTxManager{tickets: t, hist: h}
}

func (m *fakeTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	// Snapshot
	m.tickets.mu.Lock()
	tickSnap := make(map[uuid.UUID]entity.Ticket, len(m.tickets.byID))
	for id, t := range m.tickets.byID {
		tickSnap[id] = *t
	}
	tickSeq := m.tickets.seq
	m.tickets.mu.Unlock()

	m.hist.mu.Lock()
	histSnap := make(map[uuid.UUID][]entity.TicketHistory, len(m.hist.byTicket))
	for id, rows := range m.hist.byTicket {
		histSnap[id] = append([]entity.TicketHistory(nil), rows...)
	}
	m.hist.mu.Unlock()

	err := fn(ctx)
	if err != nil {
		// Rollback
		m.tickets.mu.Lock()
		m.tickets.byID = map[uuid.UUID]*entity.Ticket{}
		for id, t := range tickSnap {
			cp := t
			m.tickets.byID[id] = &cp
		}
		m.tickets.seq = tickSeq
		m.tickets.mu.Unlock()

		m.hist.mu.Lock()
		m.hist.byTicket = histSnap
		m.hist.mu.Unlock()

		m.rollback = true
	}
	return err
}

// =========== harness ===========

type harness struct {
	svc      *Service
	users    *fakeUserRepo
	cats     *fakeCategoryRepo
	tix      *fakeTicketRepo
	hist     *fakeHistoryRepo
	comments *fakeCommentRepo
	tx       *fakeTxManager

	cat    *entity.TicketCategory
	custA  *entity.User
	custB  *entity.User
	agent  *entity.User
	agent2 *entity.User
	admin  *entity.User
}

func setup() *harness {
	users := newFakeUserRepo()
	cats := newFakeCategoryRepo()
	tix := newFakeTicketRepo()
	hist := newFakeHistoryRepo()
	comments := newFakeCommentRepo(users)
	tx := newFakeTxManager(tix, hist)

	h := &harness{
		svc:   NewService(tix, cats, users, hist, comments, tx),
		users: users, cats: cats, tix: tix, hist: hist, comments: comments, tx: tx,
	}
	h.cat = cats.addActive("Technical Issue")
	h.custA = users.add(entity.RoleCustomer, "Alice")
	h.custB = users.add(entity.RoleCustomer, "Bob")
	h.agent = users.add(entity.RoleAgent, "Agent Alpha")
	h.agent2 = users.add(entity.RoleAgent, "Agent Beta")
	h.admin = users.add(entity.RoleAdmin, "Admin")
	return h
}

func mustCreate(t *testing.T, h *harness, actor *entity.User) *TicketOutput {
	t.Helper()
	out, err := h.svc.Create(context.Background(), actor, CreateTicketInput{
		Title:       "Cannot log in",
		Description: "App rejects valid credentials.",
		CategoryID:  h.cat.ID,
		Priority:    entity.TicketPriorityHigh,
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	return out
}

func assign(t *testing.T, h *harness, actor *entity.User, ticketID uuid.UUID, agent *entity.User) {
	t.Helper()
	_, err := h.svc.Assign(context.Background(), actor, ticketID, AssignInput{AgentID: &agent.ID})
	if err != nil {
		t.Fatalf("assign failed: %v", err)
	}
}

func updateStatus(t *testing.T, h *harness, actor *entity.User, ticketID uuid.UUID, next string) error {
	t.Helper()
	_, err := h.svc.UpdateStatus(context.Background(), actor, ticketID, UpdateStatusInput{Status: next})
	return err
}

// =========== tests ===========

func TestCreate_WritesCreatedHistory(t *testing.T) {
	h := setup()
	out := mustCreate(t, h, h.custA)
	rows, err := h.svc.ListHistories(context.Background(), h.custA, out.ID)
	if err != nil {
		t.Fatalf("ListHistories failed: %v", err)
	}
	if len(rows) != 1 || rows[0].Action != entity.TicketHistoryActionCreated {
		t.Fatalf("expected one created history row, got %+v", rows)
	}
}

func TestAdminAssignsAgent(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	assign(t, h, h.admin, ticket.ID, h.agent)

	rows, _ := h.svc.ListHistories(context.Background(), h.admin, ticket.ID)
	if len(rows) != 2 || rows[1].Action != entity.TicketHistoryActionAssigned {
		t.Fatalf("expected assigned history row, got %+v", rows)
	}
	stored := h.tix.byID[ticket.ID]
	if stored.AssignedTo == nil || *stored.AssignedTo != h.agent.ID {
		t.Fatalf("ticket not assigned to agent: %+v", stored)
	}
}

func TestAdminReassignsAgent(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	assign(t, h, h.admin, ticket.ID, h.agent)
	_, err := h.svc.Assign(context.Background(), h.admin, ticket.ID, AssignInput{AgentID: &h.agent2.ID})
	if err != nil {
		t.Fatalf("reassign failed: %v", err)
	}
	rows, _ := h.svc.ListHistories(context.Background(), h.admin, ticket.ID)
	if rows[len(rows)-1].Action != entity.TicketHistoryActionReassigned {
		t.Fatalf("expected reassigned action, got %v", rows[len(rows)-1].Action)
	}
}

func TestCustomerCannotAssign(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	_, err := h.svc.Assign(context.Background(), h.custA, ticket.ID, AssignInput{AgentID: &h.agent.ID})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestAssign_DestinationMustBeAgent(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	_, err := h.svc.Assign(context.Background(), h.admin, ticket.ID, AssignInput{AgentID: &h.custB.ID})
	if !errors.Is(err, domain.ErrInvalidAssignee) {
		t.Fatalf("expected ErrInvalidAssignee, got %v", err)
	}
}

func TestAgentOpenToInProgress_WhenAssigned(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	assign(t, h, h.admin, ticket.ID, h.agent)
	if err := updateStatus(t, h, h.agent, ticket.ID, entity.TicketStatusInProgress); err != nil {
		t.Fatalf("agent transition failed: %v", err)
	}
	if h.tix.byID[ticket.ID].Status != entity.TicketStatusInProgress {
		t.Fatal("status not updated")
	}
}

func TestAgentInProgressToResolved(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	assign(t, h, h.admin, ticket.ID, h.agent)
	if err := updateStatus(t, h, h.agent, ticket.ID, entity.TicketStatusInProgress); err != nil {
		t.Fatalf("inprogress failed: %v", err)
	}
	if err := updateStatus(t, h, h.agent, ticket.ID, entity.TicketStatusResolved); err != nil {
		t.Fatalf("resolved failed: %v", err)
	}
}

func TestInvalidTransition_OpenToResolved(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	assign(t, h, h.admin, ticket.ID, h.agent)
	err := updateStatus(t, h, h.agent, ticket.ID, entity.TicketStatusResolved)
	if !errors.Is(err, domain.ErrInvalidStatusTransition) {
		t.Fatalf("expected ErrInvalidStatusTransition, got %v", err)
	}
}

func TestUnassigned_OpenToInProgressRejected(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	err := updateStatus(t, h, h.admin, ticket.ID, entity.TicketStatusInProgress)
	if !errors.Is(err, domain.ErrTicketNotAssigned) {
		t.Fatalf("expected ErrTicketNotAssigned, got %v", err)
	}
}

func TestOtherAgentCannotUpdateStatus(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	assign(t, h, h.admin, ticket.ID, h.agent)
	err := updateStatus(t, h, h.agent2, ticket.ID, entity.TicketStatusInProgress)
	if !errors.Is(err, domain.ErrTicketNotFound) && !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrTicketNotFound/Forbidden, got %v", err)
	}
}

func TestCreatorCanReopenOwnClosed(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	assign(t, h, h.admin, ticket.ID, h.agent)
	for _, next := range []string{entity.TicketStatusInProgress, entity.TicketStatusResolved, entity.TicketStatusClosed} {
		if err := updateStatus(t, h, h.agent, ticket.ID, next); err != nil {
			t.Fatalf("transition %s failed: %v", next, err)
		}
	}
	if err := updateStatus(t, h, h.custA, ticket.ID, entity.TicketStatusReopened); err != nil {
		t.Fatalf("customer reopen failed: %v", err)
	}
}

func TestCreatorCannotOtherStatusChanges(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	assign(t, h, h.admin, ticket.ID, h.agent)
	err := updateStatus(t, h, h.custA, ticket.ID, entity.TicketStatusInProgress)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestStatusUpdate_WritesHistory(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	assign(t, h, h.admin, ticket.ID, h.agent)
	if err := updateStatus(t, h, h.agent, ticket.ID, entity.TicketStatusInProgress); err != nil {
		t.Fatalf("transition failed: %v", err)
	}
	rows, _ := h.svc.ListHistories(context.Background(), h.admin, ticket.ID)
	last := rows[len(rows)-1]
	if last.Action != entity.TicketHistoryActionStatusChanged {
		t.Fatalf("expected status_changed action, got %v", last.Action)
	}
	if last.OldStatus == nil || *last.OldStatus != entity.TicketStatusOpen {
		t.Fatalf("expected old_status open, got %v", last.OldStatus)
	}
	if last.NewStatus == nil || *last.NewStatus != entity.TicketStatusInProgress {
		t.Fatalf("expected new_status in_progress, got %v", last.NewStatus)
	}
}

func TestHistoryFailureRollsBackTicketUpdate(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	assign(t, h, h.admin, ticket.ID, h.agent)
	// Trip the history repo so its Create returns an error inside the tx.
	h.hist.failOn = "Create"
	err := updateStatus(t, h, h.agent, ticket.ID, entity.TicketStatusInProgress)
	if err == nil {
		t.Fatal("expected error when history insert fails")
	}
	if !h.tx.rollback {
		t.Fatal("expected tx manager to record rollback")
	}
	if got := h.tix.byID[ticket.ID].Status; got != entity.TicketStatusOpen {
		t.Fatalf("expected status reverted to open after rollback, got %q", got)
	}
}

func TestUnauthorizedUserCannotViewHistories(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	_, err := h.svc.ListHistories(context.Background(), h.custB, ticket.ID)
	if !errors.Is(err, domain.ErrTicketNotFound) {
		t.Fatalf("expected ErrTicketNotFound, got %v", err)
	}
}

func TestAgentCanViewHistoriesOfAssigned(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	assign(t, h, h.admin, ticket.ID, h.agent)
	rows, err := h.svc.ListHistories(context.Background(), h.agent, ticket.ID)
	if err != nil {
		t.Fatalf("agent should view histories: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected at least one history row")
	}
}

func TestListScope(t *testing.T) {
	h := setup()
	mineA := mustCreate(t, h, h.custA) // created by A
	otherB := mustCreate(t, h, h.custB)
	assign(t, h, h.admin, otherB.ID, h.agent)

	// Customer A: only own tickets, regardless of scope.
	out, err := h.svc.List(context.Background(), h.custA, ListInput{})
	if err != nil {
		t.Fatalf("custA list failed: %v", err)
	}
	if out.Total != 1 || out.Items[0].ID != mineA.ID {
		t.Fatalf("custA should see exactly own ticket, got total=%d", out.Total)
	}

	// Customer requesting assigned_to_me -> forbidden.
	if _, err := h.svc.List(context.Background(), h.custA, ListInput{Scope: ScopeAssignedToMe}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("custA scope=assigned_to_me should be forbidden, got %v", err)
	}

	// Agent default scope = creator OR assignee.
	out, err = h.svc.List(context.Background(), h.agent, ListInput{})
	if err != nil {
		t.Fatalf("agent default list failed: %v", err)
	}
	// Agent didn't create anything but is assigned to otherB.
	if out.Total != 1 || out.Items[0].ID != otherB.ID {
		t.Fatalf("agent default scope should yield otherB only, got total=%d", out.Total)
	}

	// Admin sees everything.
	out, _ = h.svc.List(context.Background(), h.admin, ListInput{})
	if out.Total != 2 {
		t.Fatalf("admin should see both tickets, got %d", out.Total)
	}
}
