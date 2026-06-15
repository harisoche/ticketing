package ticket

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"ticketing-api/internal/domain"
	"ticketing-api/internal/domain/entity"
)

func addComment(t *testing.T, h *harness, actor *entity.User, ticketID uuid.UUID, body string) *CommentOutput {
	t.Helper()
	out, err := h.svc.AddComment(context.Background(), actor, ticketID, AddCommentInput{Body: body})
	if err != nil {
		t.Fatalf("AddComment failed: %v", err)
	}
	return out
}

// 1. Ticket creator adds a comment.
func TestComment_CreatorCanAdd(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	out := addComment(t, h, h.custA, ticket.ID, "Hello from the creator.")
	if out.Body != "Hello from the creator." {
		t.Errorf("body mismatch: %q", out.Body)
	}
	if out.Author == nil || out.Author.ID != h.custA.ID {
		t.Errorf("author mismatch: %+v", out.Author)
	}
	if out.Author.Role != entity.RoleCustomer {
		t.Errorf("author role should be customer, got %q", out.Author.Role)
	}
}

// 2. Assigned agent adds a comment.
func TestComment_AssignedAgentCanAdd(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	assign(t, h, h.admin, ticket.ID, h.agent)
	out := addComment(t, h, h.agent, ticket.ID, "Looking into it.")
	if out.Author == nil || out.Author.ID != h.agent.ID {
		t.Errorf("expected agent as author")
	}
}

// 3. Admin adds a comment.
func TestComment_AdminCanAdd(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	out := addComment(t, h, h.admin, ticket.ID, "Admin note.")
	if out.Body != "Admin note." {
		t.Errorf("admin comment body wrong: %q", out.Body)
	}
}

// 4. Unrelated user cannot list or add comments.
func TestComment_UnrelatedUserDenied(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	_, err := h.svc.AddComment(context.Background(), h.custB, ticket.ID, AddCommentInput{Body: "x"})
	if !errors.Is(err, domain.ErrTicketNotFound) {
		t.Errorf("AddComment by unrelated user expected ErrTicketNotFound, got %v", err)
	}
	_, err = h.svc.ListComments(context.Background(), h.custB, ticket.ID)
	if !errors.Is(err, domain.ErrTicketNotFound) {
		t.Errorf("ListComments by unrelated user expected ErrTicketNotFound, got %v", err)
	}
}

// 5. Unassigned agent cannot access comments.
func TestComment_UnassignedAgentDenied(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	// Agent2 is never assigned.
	_, err := h.svc.ListComments(context.Background(), h.agent2, ticket.ID)
	if !errors.Is(err, domain.ErrTicketNotFound) {
		t.Errorf("expected ErrTicketNotFound for unassigned agent, got %v", err)
	}
}

// 6. Author edits their comment.
func TestComment_AuthorCanEdit(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	c := addComment(t, h, h.custA, ticket.ID, "original")
	out, err := h.svc.UpdateComment(context.Background(), h.custA, ticket.ID, c.ID, UpdateCommentInput{Body: "edited"})
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if out.Body != "edited" {
		t.Errorf("expected edited body, got %q", out.Body)
	}
}

// 7. Non-author cannot edit another user's comment.
func TestComment_NonAuthorCannotEdit(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	c := addComment(t, h, h.custA, ticket.ID, "original")
	// Make agent assigned so they can view the ticket but not edit foreign comment.
	assign(t, h, h.admin, ticket.ID, h.agent)
	_, err := h.svc.UpdateComment(context.Background(), h.agent, ticket.ID, c.ID, UpdateCommentInput{Body: "no"})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("expected ErrForbidden, got %v", err)
	}
}

// 8. Admin edits another user's comment.
func TestComment_AdminCanEditAny(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	c := addComment(t, h, h.custA, ticket.ID, "original")
	out, err := h.svc.UpdateComment(context.Background(), h.admin, ticket.ID, c.ID, UpdateCommentInput{Body: "moderated"})
	if err != nil {
		t.Fatalf("admin edit failed: %v", err)
	}
	if out.Body != "moderated" {
		t.Errorf("admin edit didn't apply, got %q", out.Body)
	}
}

// 9. Author deletes their comment.
func TestComment_AuthorCanDelete(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	c := addComment(t, h, h.custA, ticket.ID, "delete me")
	if err := h.svc.DeleteComment(context.Background(), h.custA, ticket.ID, c.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	rows, _ := h.svc.ListComments(context.Background(), h.custA, ticket.ID)
	if len(rows) != 0 {
		t.Errorf("expected zero comments after delete, got %d", len(rows))
	}
}

// 10. Admin deletes another user's comment.
func TestComment_AdminCanDeleteAny(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	c := addComment(t, h, h.custA, ticket.ID, "x")
	if err := h.svc.DeleteComment(context.Background(), h.admin, ticket.ID, c.ID); err != nil {
		t.Fatalf("admin delete failed: %v", err)
	}
}

// 11. Comment ID must belong to the path ticket.
func TestComment_PathTicketMismatch(t *testing.T) {
	h := setup()
	t1 := mustCreate(t, h, h.custA)
	t2 := mustCreate(t, h, h.custA)
	c := addComment(t, h, h.custA, t1.ID, "for ticket 1")

	// Try updating with t2 in the path.
	_, err := h.svc.UpdateComment(context.Background(), h.custA, t2.ID, c.ID, UpdateCommentInput{Body: "no"})
	if !errors.Is(err, domain.ErrCommentNotFound) {
		t.Errorf("expected ErrCommentNotFound for path mismatch, got %v", err)
	}
	if err := h.svc.DeleteComment(context.Background(), h.custA, t2.ID, c.ID); !errors.Is(err, domain.ErrCommentNotFound) {
		t.Errorf("expected ErrCommentNotFound on delete with wrong path, got %v", err)
	}
}

// 12. Blank body is rejected after trimming.
func TestComment_BlankBodyRejected(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	for _, body := range []string{"", "   ", "\n\t"} {
		_, err := h.svc.AddComment(context.Background(), h.custA, ticket.ID, AddCommentInput{Body: body})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("body %q: expected ErrInvalidInput, got %v", body, err)
		}
	}
	// Too long
	_, err := h.svc.AddComment(context.Background(), h.custA, ticket.ID, AddCommentInput{Body: strings.Repeat("a", 5001)})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("oversize body: expected ErrInvalidInput, got %v", err)
	}
}

// 13. Timeline contains both history and comment items.
func TestTimeline_ContainsHistoryAndComment(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	assign(t, h, h.admin, ticket.ID, h.agent)
	addComment(t, h, h.custA, ticket.ID, "first")

	items, err := h.svc.GetTimeline(context.Background(), h.admin, ticket.ID)
	if err != nil {
		t.Fatalf("GetTimeline failed: %v", err)
	}
	if len(items) < 3 { // created + assigned + comment
		t.Fatalf("expected at least 3 timeline items, got %d", len(items))
	}
	var sawHistory, sawComment bool
	for _, it := range items {
		switch it.Type {
		case "history":
			sawHistory = true
			if it.History == nil {
				t.Error("history item missing payload")
			}
			if it.Comment != nil {
				t.Error("history item should not include comment payload")
			}
		case "comment":
			sawComment = true
			if it.Comment == nil {
				t.Error("comment item missing payload")
			}
			if it.History != nil {
				t.Error("comment item should not include history payload")
			}
		default:
			t.Errorf("unknown timeline type: %q", it.Type)
		}
	}
	if !sawHistory || !sawComment {
		t.Errorf("timeline missing both kinds: history=%v comment=%v", sawHistory, sawComment)
	}
}

// 14. Timeline sorting is ascending and stable.
func TestTimeline_SortAscendingStable(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	assign(t, h, h.admin, ticket.ID, h.agent)
	addComment(t, h, h.custA, ticket.ID, "first comment")
	addComment(t, h, h.agent, ticket.ID, "second comment")
	if err := updateStatus(t, h, h.agent, ticket.ID, entity.TicketStatusInProgress); err != nil {
		t.Fatalf("transition failed: %v", err)
	}

	items, err := h.svc.GetTimeline(context.Background(), h.admin, ticket.ID)
	if err != nil {
		t.Fatalf("GetTimeline failed: %v", err)
	}
	for i := 1; i < len(items); i++ {
		prev, cur := items[i-1], items[i]
		if cur.OccurredAt.Before(prev.OccurredAt) {
			t.Errorf("timeline not ascending at %d: %v before %v", i, cur.OccurredAt, prev.OccurredAt)
		}
	}
}

// 15. User output never exposes sensitive user fields.
func TestComment_UserSummaryHasNoSensitiveFields(t *testing.T) {
	h := setup()
	ticket := mustCreate(t, h, h.custA)
	c := addComment(t, h, h.custA, ticket.ID, "Hi")
	// Inspect the JSON shape of UserSummary using reflection on the struct itself.
	got := reflect.ValueOf(*c.Author).Type()
	for i := 0; i < got.NumField(); i++ {
		f := got.Field(i)
		name := strings.ToLower(f.Name)
		if strings.Contains(name, "password") || strings.Contains(name, "hash") || strings.Contains(name, "secret") {
			t.Errorf("UserSummary leaks sensitive field %q", f.Name)
		}
	}
	// Also confirm role is filled (so we know we're not silently empty).
	if c.Author.Role == "" {
		t.Errorf("expected author role populated, got empty")
	}
}
