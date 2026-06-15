package ticket

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"ticketing-api/internal/domain"
	"ticketing-api/internal/domain/entity"
	"ticketing-api/internal/domain/repository"
)

// ---------- Phase 7 fakes ----------

type fakeSLAPolicyRepo struct {
	mu       sync.Mutex
	byPrio   map[string]*entity.SLAPolicy
	inactive map[string]bool
}

func newFakeSLAPolicyRepo() *fakeSLAPolicyRepo {
	return &fakeSLAPolicyRepo{byPrio: map[string]*entity.SLAPolicy{}, inactive: map[string]bool{}}
}

func (r *fakeSLAPolicyRepo) seed(priority string, response, resolution int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byPrio[priority] = &entity.SLAPolicy{
		ID: uuid.New(), Priority: priority,
		ResponseMinutes: response, ResolutionMinutes: resolution, IsActive: true,
	}
}

func (r *fakeSLAPolicyRepo) FindActiveByPriority(_ context.Context, priority string) (*entity.SLAPolicy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.byPrio[priority]
	if !ok || r.inactive[priority] {
		return nil, domain.ErrSLAPolicyNotFound
	}
	clone := *p
	return &clone, nil
}

type fakeNotificationRepo struct {
	mu     sync.Mutex
	rows   []entity.Notification
	failOn string // "" | "CreateMany"
}

func newFakeNotificationRepo() *fakeNotificationRepo {
	return &fakeNotificationRepo{}
}

func (r *fakeNotificationRepo) CreateMany(_ context.Context, ns []entity.Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failOn == "CreateMany" {
		return errors.New("induced notification failure")
	}
	for i := range ns {
		if ns[i].ID == uuid.Nil {
			ns[i].ID = uuid.New()
		}
		ns[i].CreatedAt = time.Now().UTC()
		r.rows = append(r.rows, ns[i])
	}
	return nil
}

func (r *fakeNotificationRepo) ListByRecipient(_ context.Context, recipientID int64, f repository.NotificationListFilter) ([]entity.Notification, int64, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	matches := []entity.Notification{}
	unread := int64(0)
	for _, n := range r.rows {
		if n.RecipientID != recipientID {
			continue
		}
		if n.ReadAt == nil {
			unread++
		}
		if f.UnreadOnly && n.ReadAt != nil {
			continue
		}
		matches = append(matches, n)
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].CreatedAt.After(matches[j].CreatedAt) })
	return matches, int64(len(matches)), unread, nil
}

func (r *fakeNotificationRepo) FindByID(_ context.Context, id uuid.UUID) (*entity.Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.rows {
		if r.rows[i].ID == id {
			clone := r.rows[i]
			return &clone, nil
		}
	}
	return nil, domain.ErrNotificationNotFound
}

func (r *fakeNotificationRepo) MarkRead(_ context.Context, recipientID int64, id uuid.UUID, readAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.rows {
		if r.rows[i].ID == id {
			if r.rows[i].RecipientID != recipientID {
				return domain.ErrNotificationNotFound
			}
			if r.rows[i].ReadAt == nil {
				cp := readAt
				r.rows[i].ReadAt = &cp
			}
			return nil
		}
	}
	return domain.ErrNotificationNotFound
}

func (r *fakeNotificationRepo) MarkAllRead(_ context.Context, recipientID int64, readAt time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for i := range r.rows {
		if r.rows[i].RecipientID == recipientID && r.rows[i].ReadAt == nil {
			cp := readAt
			r.rows[i].ReadAt = &cp
			n++
		}
	}
	return n, nil
}

func (r *fakeNotificationRepo) UnreadCount(_ context.Context, recipientID int64) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	for _, row := range r.rows {
		if row.RecipientID == recipientID && row.ReadAt == nil {
			n++
		}
	}
	return n, nil
}

// ---------- phase 7 harness ----------

type phase7Harness struct {
	*harness
	policies *fakeSLAPolicyRepo
	notifs   *fakeNotificationRepo
	clock    *time.Time
}

func setupPhase7(t *testing.T) *phase7Harness {
	t.Helper()
	pol := newFakeSLAPolicyRepo()
	pol.seed("low", 480, 2880)
	pol.seed("medium", 240, 1440)
	pol.seed("high", 60, 480)
	pol.seed("urgent", 15, 120)
	notifs := newFakeNotificationRepo()

	h := setup()
	now := time.Date(2026, 6, 5, 8, 0, 0, 0, time.UTC)
	clockPtr := &now
	clockFn := func() time.Time { return *clockPtr }
	h.tix.clock = clockFn
	h.svc = h.svc.WithPhase7(pol, notifs).WithClock(clockFn)
	return &phase7Harness{harness: h, policies: pol, notifs: notifs, clock: clockPtr}
}

func (h *phase7Harness) advance(d time.Duration) {
	next := h.clock.Add(d)
	*h.clock = next
}

// ---------- tests ----------

// 1. Ticket creation calculates SLA timestamps from priority policy.
func TestP7Create_SetsDueTimes(t *testing.T) {
	h := setupPhase7(t)
	out, err := h.svc.Create(context.Background(), h.custA, CreateTicketInput{
		Title:       "Test ticket",
		Description: "Long enough description.",
		CategoryID:  h.cat.ID,
		Priority:    entity.TicketPriorityHigh,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out.SLA == nil {
		t.Fatal("expected SLA block")
	}
	if out.SLA.ResponseDueAt == nil || out.SLA.ResolutionDueAt == nil {
		t.Fatal("due times not populated")
	}
	stored := h.tix.byID[out.ID]
	wantResp := stored.CreatedAt.Add(60 * time.Minute)
	if !stored.ResponseDueAt.Equal(wantResp) {
		t.Errorf("response_due_at: want %v got %v", wantResp, stored.ResponseDueAt)
	}
}

// 2. Classification priority update recalculates due timestamps from
// original creation time.
func TestP7Classify_RecalculatesDueTimes(t *testing.T) {
	h := setupPhase7(t)
	created := mustCreate(t, h.harness, h.custA)
	stored := h.tix.byID[created.ID]
	originalCreatedAt := stored.CreatedAt

	assign(t, h.harness, h.admin, created.ID, h.agent)

	urgent := entity.TicketPriorityUrgent
	_, err := h.svc.ClassifyTicket(context.Background(), h.agent, created.ID, ClassifyTicketInput{Priority: &urgent})
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	stored = h.tix.byID[created.ID]
	wantResp := originalCreatedAt.Add(15 * time.Minute)
	if !stored.ResponseDueAt.Equal(wantResp) {
		t.Errorf("response_due_at after reclass: want %v got %v", wantResp, stored.ResponseDueAt)
	}
}

// 3. First assigned-agent comment sets first_responded_at once.
func TestP7AgentComment_SetsFirstResponse(t *testing.T) {
	h := setupPhase7(t)
	ticket := mustCreate(t, h.harness, h.custA)
	assign(t, h.harness, h.admin, ticket.ID, h.agent)

	h.advance(30 * time.Minute)
	if _, err := h.svc.AddComment(context.Background(), h.agent, ticket.ID, AddCommentInput{Body: "looking into it"}); err != nil {
		t.Fatalf("agent comment: %v", err)
	}
	stored := h.tix.byID[ticket.ID]
	if stored.FirstRespondedAt == nil {
		t.Fatal("first_responded_at not set")
	}
	first := *stored.FirstRespondedAt

	h.advance(10 * time.Minute)
	if _, err := h.svc.AddComment(context.Background(), h.agent, ticket.ID, AddCommentInput{Body: "more"}); err != nil {
		t.Fatalf("second agent comment: %v", err)
	}
	stored = h.tix.byID[ticket.ID]
	if !stored.FirstRespondedAt.Equal(first) {
		t.Errorf("first_responded_at overwritten: want %v got %v", first, stored.FirstRespondedAt)
	}
}

// 4. Creator comment does not count as a first response.
func TestP7CreatorComment_DoesNotCount(t *testing.T) {
	h := setupPhase7(t)
	ticket := mustCreate(t, h.harness, h.custA)

	if _, err := h.svc.AddComment(context.Background(), h.custA, ticket.ID, AddCommentInput{Body: "hello"}); err != nil {
		t.Fatalf("creator comment: %v", err)
	}
	stored := h.tix.byID[ticket.ID]
	if stored.FirstRespondedAt != nil {
		t.Errorf("creator comment must not set first_responded_at")
	}
}

// 5. Moving to in_progress sets first response when null.
func TestP7Status_InProgressSetsFirstResponse(t *testing.T) {
	h := setupPhase7(t)
	ticket := mustCreate(t, h.harness, h.custA)
	assign(t, h.harness, h.admin, ticket.ID, h.agent)

	h.advance(20 * time.Minute)
	if err := updateStatus(t, h.harness, h.agent, ticket.ID, entity.TicketStatusInProgress); err != nil {
		t.Fatalf("in_progress: %v", err)
	}
	stored := h.tix.byID[ticket.ID]
	if stored.FirstRespondedAt == nil {
		t.Fatal("first_responded_at not set after in_progress")
	}
}

// 6. Moving to resolved sets resolved_at.
func TestP7Status_ResolvedSetsResolvedAt(t *testing.T) {
	h := setupPhase7(t)
	ticket := mustCreate(t, h.harness, h.custA)
	assign(t, h.harness, h.admin, ticket.ID, h.agent)
	if err := updateStatus(t, h.harness, h.agent, ticket.ID, entity.TicketStatusInProgress); err != nil {
		t.Fatalf("in_progress: %v", err)
	}
	h.advance(5 * time.Minute)
	if err := updateStatus(t, h.harness, h.agent, ticket.ID, entity.TicketStatusResolved); err != nil {
		t.Fatalf("resolved: %v", err)
	}
	stored := h.tix.byID[ticket.ID]
	if stored.ResolvedAt == nil {
		t.Fatal("resolved_at not set")
	}
}

// 7. Reopening clears resolved and closed timestamps.
func TestP7Status_ReopenClearsTimestamps(t *testing.T) {
	h := setupPhase7(t)
	ticket := mustCreate(t, h.harness, h.custA)
	assign(t, h.harness, h.admin, ticket.ID, h.agent)
	for _, next := range []string{entity.TicketStatusInProgress, entity.TicketStatusResolved, entity.TicketStatusClosed} {
		if err := updateStatus(t, h.harness, h.agent, ticket.ID, next); err != nil {
			t.Fatalf("transition %s: %v", next, err)
		}
	}
	stored := h.tix.byID[ticket.ID]
	if stored.ClosedAt == nil {
		t.Fatal("closed_at should be set")
	}

	if err := updateStatus(t, h.harness, h.custA, ticket.ID, entity.TicketStatusReopened); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	stored = h.tix.byID[ticket.ID]
	if stored.ResolvedAt != nil || stored.ClosedAt != nil {
		t.Errorf("reopen should clear resolved_at and closed_at")
	}
}

// 8 & 9. Response/Resolution SLA states use the injected clock.
func TestP7SLA_StatesAcrossClock(t *testing.T) {
	h := setupPhase7(t)
	ticket := mustCreate(t, h.harness, h.custA)
	// SLA pending immediately after creation.
	out, _ := h.svc.Get(context.Background(), h.custA, ticket.ID)
	if out.SLA == nil || out.SLA.ResponseState != SLAStatePending || out.SLA.ResolutionState != SLAStatePending {
		t.Errorf("expected pending states, got %+v", out.SLA)
	}

	// Move past the response deadline without any first response: breached.
	h.advance(120 * time.Minute) // medium=240min response → still within
	h.advance(140 * time.Minute) // now > 240 min
	out, _ = h.svc.Get(context.Background(), h.custA, ticket.ID)
	if out.SLA.ResponseState != SLAStateBreached {
		t.Errorf("response state should be breached, got %s", out.SLA.ResponseState)
	}

	// Assign+respond in-progress -> response state turns met because the
	// recorded first_responded_at is *after* the deadline → also breached.
	// (Spec: "first response after deadline" = breached.)
	assign(t, h.harness, h.admin, ticket.ID, h.agent)
	if err := updateStatus(t, h.harness, h.agent, ticket.ID, entity.TicketStatusInProgress); err != nil {
		t.Fatalf("in_progress: %v", err)
	}
	out, _ = h.svc.Get(context.Background(), h.custA, ticket.ID)
	if out.SLA.ResponseState != SLAStateBreached {
		t.Errorf("late first response should still breach, got %s", out.SLA.ResponseState)
	}
	if !out.SLA.IsResolutionOverdue && out.SLA.ResolutionDueAt != nil && h.clock.After(*out.SLA.ResolutionDueAt) {
		t.Errorf("expected resolution overdue flag")
	}
}

// 10. Assignment creates agent notification.
func TestP7Notif_AssignmentNotifiesAgent(t *testing.T) {
	h := setupPhase7(t)
	ticket := mustCreate(t, h.harness, h.custA)
	assign(t, h.harness, h.admin, ticket.ID, h.agent)
	found := false
	for _, n := range h.notifs.rows {
		if n.RecipientID == h.agent.ID && n.Type == entity.NotificationTypeTicketAssigned {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ticket_assigned notification for agent")
	}
}

// 11. Comment by creator notifies assigned agent.
func TestP7Notif_CreatorCommentNotifiesAgent(t *testing.T) {
	h := setupPhase7(t)
	ticket := mustCreate(t, h.harness, h.custA)
	assign(t, h.harness, h.admin, ticket.ID, h.agent)

	h.notifs.rows = nil
	if _, err := h.svc.AddComment(context.Background(), h.custA, ticket.ID, AddCommentInput{Body: "still broken"}); err != nil {
		t.Fatalf("comment: %v", err)
	}
	if len(h.notifs.rows) != 1 || h.notifs.rows[0].RecipientID != h.agent.ID || h.notifs.rows[0].Type != entity.NotificationTypeTicketCommented {
		t.Errorf("expected one comment notification to agent, got %+v", h.notifs.rows)
	}
}

// 12. Agent comment notifies creator.
func TestP7Notif_AgentCommentNotifiesCreator(t *testing.T) {
	h := setupPhase7(t)
	ticket := mustCreate(t, h.harness, h.custA)
	assign(t, h.harness, h.admin, ticket.ID, h.agent)

	h.notifs.rows = nil
	if _, err := h.svc.AddComment(context.Background(), h.agent, ticket.ID, AddCommentInput{Body: "looking"}); err != nil {
		t.Fatalf("comment: %v", err)
	}
	if len(h.notifs.rows) != 1 || h.notifs.rows[0].RecipientID != h.custA.ID {
		t.Errorf("expected one comment notification to creator, got %+v", h.notifs.rows)
	}
}

// 13. Self-notification is not created.
//
// Scenarios actually possible in this project (admin can't self-assign
// because assignee must have role=agent):
//   - admin commenting on a ticket they themselves created → no creator
//     notification (recipient would be the actor).
//   - agent assigned to a ticket they themselves created and then
//     transitioning the status: no creator notification because
//     actor == creator.
func TestP7Notif_NoSelfNotify(t *testing.T) {
	h := setupPhase7(t)

	tic, _ := h.svc.Create(context.Background(), h.admin, CreateTicketInput{
		Title: "Admin's own", Description: "long enough description", CategoryID: h.cat.ID, Priority: "medium",
	})
	h.notifs.rows = nil
	if _, err := h.svc.AddComment(context.Background(), h.admin, tic.ID, AddCommentInput{Body: "self note"}); err != nil {
		t.Fatalf("comment: %v", err)
	}
	for _, n := range h.notifs.rows {
		if n.RecipientID == h.admin.ID {
			t.Errorf("admin commenting on own ticket should not self-notify; got %+v", n)
		}
	}

	tic2, _ := h.svc.Create(context.Background(), h.agent, CreateTicketInput{
		Title: "Agent's own", Description: "long enough description", CategoryID: h.cat.ID, Priority: "medium",
	})
	assign(t, h.harness, h.admin, tic2.ID, h.agent)
	h.notifs.rows = nil
	if err := updateStatus(t, h.harness, h.agent, tic2.ID, entity.TicketStatusInProgress); err != nil {
		t.Fatalf("in_progress: %v", err)
	}
	for _, n := range h.notifs.rows {
		if n.RecipientID == h.agent.ID && n.Type == entity.NotificationTypeTicketStatusChanged {
			t.Errorf("agent transitioning own ticket should not self-notify; got %+v", n)
		}
	}
}

// 14. Notification insert failure rolls back triggering transaction.
func TestP7Notif_FailureRollsBack(t *testing.T) {
	h := setupPhase7(t)
	ticket := mustCreate(t, h.harness, h.custA)
	assign(t, h.harness, h.admin, ticket.ID, h.agent)

	h.notifs.failOn = "CreateMany"
	err := updateStatus(t, h.harness, h.agent, ticket.ID, entity.TicketStatusInProgress)
	if err == nil {
		t.Fatal("expected error when notification insert fails")
	}
	if !h.tx.rollback {
		t.Fatal("expected tx rollback")
	}
	if h.tix.byID[ticket.ID].Status != entity.TicketStatusOpen {
		t.Errorf("status should remain open after rollback, got %s", h.tix.byID[ticket.ID].Status)
	}
}

// 15. Notification list returns only actor notifications.
func TestP7Notif_ListScopedToActor(t *testing.T) {
	h := setupPhase7(t)
	// Generate one notification each for custA and custB.
	ticket := mustCreate(t, h.harness, h.custA)
	assign(t, h.harness, h.admin, ticket.ID, h.agent)
	ticket2 := mustCreate(t, h.harness, h.custB)
	assign(t, h.harness, h.admin, ticket2.ID, h.agent)

	out, err := h.svc.ListNotifications(context.Background(), h.agent, NotificationListInput{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// We expect the agent to see at least the two assignment notifications.
	if out.Total < 2 {
		t.Errorf("agent should see at least 2 notifications, got %d", out.Total)
	}
	// The creator should NOT see the agent's notifications.
	out, _ = h.svc.ListNotifications(context.Background(), h.custB, NotificationListInput{})
	for _, n := range out.Items {
		if n.Type == entity.NotificationTypeTicketAssigned {
			t.Errorf("creator should not see assignment notification: %+v", n)
		}
	}
}

// 16. Actor cannot mark another user's notification as read.
func TestP7Notif_CrossUserMarkReadDenied(t *testing.T) {
	h := setupPhase7(t)
	ticket := mustCreate(t, h.harness, h.custA)
	assign(t, h.harness, h.admin, ticket.ID, h.agent)

	// agent has one notification
	var note entity.Notification
	for _, n := range h.notifs.rows {
		if n.RecipientID == h.agent.ID {
			note = n
			break
		}
	}
	if note.ID == uuid.Nil {
		t.Fatal("expected an agent notification to exist")
	}

	if err := h.svc.MarkNotificationRead(context.Background(), h.custA, note.ID); !errors.Is(err, domain.ErrNotificationNotFound) {
		t.Errorf("expected ErrNotificationNotFound when marking foreign notification, got %v", err)
	}
}

// 17. Mark-read is idempotent.
func TestP7Notif_MarkReadIdempotent(t *testing.T) {
	h := setupPhase7(t)
	ticket := mustCreate(t, h.harness, h.custA)
	assign(t, h.harness, h.admin, ticket.ID, h.agent)
	var note entity.Notification
	for _, n := range h.notifs.rows {
		if n.RecipientID == h.agent.ID {
			note = n
			break
		}
	}
	if err := h.svc.MarkNotificationRead(context.Background(), h.agent, note.ID); err != nil {
		t.Fatalf("mark: %v", err)
	}
	// Second call must not error.
	if err := h.svc.MarkNotificationRead(context.Background(), h.agent, note.ID); err != nil {
		t.Errorf("second mark-read should be idempotent: %v", err)
	}
}

// 18. Dashboard SLA metrics respect ticket visibility.
func TestP7Dashboard_VisibilityAware(t *testing.T) {
	h := setupPhase7(t)
	// Create three tickets owned by custA, one owned by custB.
	for i := 0; i < 3; i++ {
		mustCreate(t, h.harness, h.custA)
	}
	mustCreate(t, h.harness, h.custB)

	// Move clock past medium's response deadline so all four are breached.
	h.advance(241 * time.Minute)

	out, err := h.svc.DashboardSummary(context.Background(), h.custA)
	if err != nil {
		t.Fatalf("dashboard: %v", err)
	}
	if out.SLA.ResponseBreached != 3 {
		t.Errorf("custA should see 3 breached, got %d", out.SLA.ResponseBreached)
	}
	out, err = h.svc.DashboardSummary(context.Background(), h.admin)
	if err != nil {
		t.Fatalf("dashboard admin: %v", err)
	}
	if out.SLA.ResponseBreached != 4 {
		t.Errorf("admin should see 4 breached, got %d", out.SLA.ResponseBreached)
	}
}
