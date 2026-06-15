package ticket

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"ticketing-api/internal/domain"
	"ticketing-api/internal/domain/entity"
	"ticketing-api/internal/domain/repository"
)

// 1. Default pagination values.
func TestList_DefaultPagination(t *testing.T) {
	h := setup()
	mustCreate(t, h, h.custA)
	out, err := h.svc.List(context.Background(), h.custA, ListInput{})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if out.Page != 1 {
		t.Errorf("expected default page=1, got %d", out.Page)
	}
	if out.PerPage != defaultPerPage {
		t.Errorf("expected default per_page=%d, got %d", defaultPerPage, out.PerPage)
	}
}

// 2. Maximum per_page enforcement.
func TestList_PerPageClamped(t *testing.T) {
	h := setup()
	mustCreate(t, h, h.custA)
	out, err := h.svc.List(context.Background(), h.custA, ListInput{PerPage: 500})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if out.PerPage != maxPerPage {
		t.Errorf("expected per_page clamped to %d, got %d", maxPerPage, out.PerPage)
	}
}

// 3. Invalid page returns validation error.
func TestList_InvalidPageRejected(t *testing.T) {
	h := setup()
	_, err := h.svc.List(context.Background(), h.custA, ListInput{Page: -1})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for page=-1, got %v", err)
	}
	_, err = h.svc.List(context.Background(), h.custA, ListInput{PerPage: -1})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for per_page=-1, got %v", err)
	}
}

// 4. Keyword (`q`) is passed safely to the repository.
func TestList_QueryPassedToRepo(t *testing.T) {
	h := setup()
	mustCreate(t, h, h.custA)
	// Wrap the underlying fake repo to capture the param.
	prev := h.svc.tickets
	cap := &captureListRepo{inner: prev}
	h.svc.tickets = cap
	_, err := h.svc.List(context.Background(), h.custA, ListInput{Query: "  wifi room a  "})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if cap.lastQuery != "wifi room a" {
		t.Errorf("expected trimmed q passed through, got %q", cap.lastQuery)
	}
}

// 5. Unsupported sort field is rejected.
func TestList_UnknownSortRejected(t *testing.T) {
	h := setup()
	_, err := h.svc.List(context.Background(), h.custA, ListInput{SortBy: "garbage"})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for sort_by=garbage, got %v", err)
	}
	// status was added in Phase 6.
	if _, err := h.svc.List(context.Background(), h.custA, ListInput{SortBy: "status"}); err != nil {
		t.Errorf("status should be allowed, got %v", err)
	}
}

// 6. Customer cannot broaden visibility through query parameters.
func TestList_CustomerCannotBroaden(t *testing.T) {
	h := setup()
	mineA := mustCreate(t, h, h.custA)
	mineB := mustCreate(t, h, h.custB)
	// custA passes assigned_to=<custB.ID> and created_by=<custB.ID>: both
	// must be ignored. The visibility scope wins.
	otherID := h.custB.ID
	out, err := h.svc.List(context.Background(), h.custA, ListInput{
		CreatedBy: &otherID, AssignedTo: &otherID,
	})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if out.Total != 1 || out.Items[0].ID != mineA.ID {
		t.Errorf("customer should still see only own ticket, got total=%d", out.Total)
	}
	_ = mineB
	// view=all must be rejected for customer.
	if _, err := h.svc.List(context.Background(), h.custA, ListInput{View: ViewAll}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("customer view=all should fail with ErrInvalidInput, got %v", err)
	}
}

// 7. Agent assigned_to_me returns assigned tickets.
func TestList_AgentAssignedView(t *testing.T) {
	h := setup()
	t1 := mustCreate(t, h, h.custA)
	mustCreate(t, h, h.custB) // not assigned to agent
	assign(t, h, h.admin, t1.ID, h.agent)

	out, err := h.svc.List(context.Background(), h.agent, ListInput{View: ScopeAssignedToMe})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if out.Total != 1 || out.Items[0].ID != t1.ID {
		t.Errorf("agent assigned_to_me should yield t1 only, got total=%d", out.Total)
	}

	// agent view=all is forbidden.
	if _, err := h.svc.List(context.Background(), h.agent, ListInput{View: ViewAll}); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("agent view=all should fail with ErrForbidden, got %v", err)
	}
}

// 8. Admin filters by creator and assignee.
func TestList_AdminFiltersByCreatorAndAssignee(t *testing.T) {
	h := setup()
	t1 := mustCreate(t, h, h.custA)
	mustCreate(t, h, h.custB)
	assign(t, h, h.admin, t1.ID, h.agent)

	creatorID := h.custA.ID
	out, err := h.svc.List(context.Background(), h.admin, ListInput{CreatedBy: &creatorID})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if out.Total != 1 || out.Items[0].ID != t1.ID {
		t.Errorf("admin created_by=custA should yield t1 only, got total=%d", out.Total)
	}

	agentID := h.agent.ID
	out, err = h.svc.List(context.Background(), h.admin, ListInput{AssignedTo: &agentID})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if out.Total != 1 || out.Items[0].ID != t1.ID {
		t.Errorf("admin assigned_to=agent should yield t1 only, got total=%d", out.Total)
	}
}

// 9. Date range parsing + validation.
func TestList_DateRangeValidation(t *testing.T) {
	h := setup()
	from := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) // before from
	_, err := h.svc.List(context.Background(), h.admin, ListInput{CreatedFrom: &from, CreatedTo: &to})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("expected ErrInvalidInput for from>to, got %v", err)
	}
}

// 10. List total count is independent of page size.
func TestList_TotalIndependentOfPage(t *testing.T) {
	h := setup()
	for i := 0; i < 3; i++ {
		mustCreate(t, h, h.custA)
	}
	a, _ := h.svc.List(context.Background(), h.custA, ListInput{PerPage: 1})
	b, _ := h.svc.List(context.Background(), h.custA, ListInput{PerPage: 100})
	if a.Total != 3 || b.Total != 3 {
		t.Errorf("total should be 3 regardless of per_page; got %d and %d", a.Total, b.Total)
	}
}

// 11. Empty list returns empty data and valid meta.
func TestList_EmptyMetaValid(t *testing.T) {
	h := setup()
	out, err := h.svc.List(context.Background(), h.custA, ListInput{})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(out.Items) != 0 {
		t.Errorf("expected empty items, got %d", len(out.Items))
	}
	if out.Total != 0 {
		t.Errorf("expected total=0, got %d", out.Total)
	}
	if out.Page != 1 || out.PerPage != defaultPerPage {
		t.Errorf("expected default meta page=1 per_page=%d, got %d/%d", defaultPerPage, out.Page, out.PerPage)
	}
}

// 12. Dashboard customer counts only owned tickets.
func TestDashboard_CustomerOwnedOnly(t *testing.T) {
	h := setup()
	mustCreate(t, h, h.custA)
	mustCreate(t, h, h.custA)
	mustCreate(t, h, h.custB)

	out, err := h.svc.DashboardSummary(context.Background(), h.custA)
	if err != nil {
		t.Fatalf("dashboard failed: %v", err)
	}
	if out.TotalTickets != 2 {
		t.Errorf("custA should see 2, got %d", out.TotalTickets)
	}
}

// 13. Dashboard agent counts only accessible tickets.
func TestDashboard_AgentAccessibleOnly(t *testing.T) {
	h := setup()
	t1 := mustCreate(t, h, h.custA)
	t2 := mustCreate(t, h, h.custB)
	mustCreate(t, h, h.custB)
	assign(t, h, h.admin, t1.ID, h.agent)

	out, err := h.svc.DashboardSummary(context.Background(), h.agent)
	if err != nil {
		t.Fatalf("dashboard failed: %v", err)
	}
	if out.TotalTickets != 1 {
		t.Errorf("agent should see 1 (assigned), got %d", out.TotalTickets)
	}
	_ = t2
}

// 14. Dashboard admin counts all tickets.
func TestDashboard_AdminAllTickets(t *testing.T) {
	h := setup()
	mustCreate(t, h, h.custA)
	mustCreate(t, h, h.custB)
	mustCreate(t, h, h.custA)

	out, err := h.svc.DashboardSummary(context.Background(), h.admin)
	if err != nil {
		t.Fatalf("dashboard failed: %v", err)
	}
	if out.TotalTickets != 3 {
		t.Errorf("admin should see 3, got %d", out.TotalTickets)
	}
}

// 15. Dashboard includes zero keys for unused statuses and priorities.
func TestDashboard_ZeroKeysIncluded(t *testing.T) {
	h := setup()
	out, err := h.svc.DashboardSummary(context.Background(), h.admin)
	if err != nil {
		t.Fatalf("dashboard failed: %v", err)
	}
	for _, key := range []string{"open", "in_progress", "resolved", "closed", "reopened"} {
		if _, ok := out.ByStatus[key]; !ok {
			t.Errorf("by_status missing zero key %q", key)
		}
	}
	for _, key := range []string{"low", "medium", "high", "urgent"} {
		if _, ok := out.ByPriority[key]; !ok {
			t.Errorf("by_priority missing zero key %q", key)
		}
	}
}

// ---------- helper repo wrapper ----------

// captureListRepo wraps the real fake to assert on the param struct that
// reaches the repository layer.
type captureListRepo struct {
	inner     repository.TicketRepository
	lastQuery string
}

func (r *captureListRepo) Create(ctx context.Context, t *entity.Ticket) error {
	return r.inner.Create(ctx, t)
}
func (r *captureListRepo) FindByID(ctx context.Context, id uuid.UUID) (*entity.Ticket, error) {
	return r.inner.FindByID(ctx, id)
}
func (r *captureListRepo) FindByIDForUpdate(ctx context.Context, id uuid.UUID) (*entity.Ticket, error) {
	return r.inner.FindByIDForUpdate(ctx, id)
}
func (r *captureListRepo) List(ctx context.Context, p repository.TicketListParam) ([]entity.Ticket, int64, error) {
	r.lastQuery = p.Query
	return r.inner.List(ctx, p)
}
func (r *captureListRepo) Update(ctx context.Context, t *entity.Ticket) error {
	return r.inner.Update(ctx, t)
}
func (r *captureListRepo) UpdateAssignment(ctx context.Context, t *entity.Ticket) error {
	return r.inner.UpdateAssignment(ctx, t)
}
func (r *captureListRepo) UpdateStatus(ctx context.Context, t *entity.Ticket) error {
	return r.inner.UpdateStatus(ctx, t)
}
func (r *captureListRepo) SoftDelete(ctx context.Context, id uuid.UUID) error {
	return r.inner.SoftDelete(ctx, id)
}
func (r *captureListRepo) Summary(ctx context.Context, scope repository.TicketListScope, now time.Time, dueSoon int) (*repository.TicketSummary, error) {
	return r.inner.Summary(ctx, scope, now, dueSoon)
}
