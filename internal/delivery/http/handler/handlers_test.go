package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"

	"ticketing-api/internal/delivery/http/response"
	"ticketing-api/internal/delivery/http/validation"
	"ticketing-api/internal/domain"
)

// newEcho builds an Echo instance configured exactly like the production
// router so handler validation behaves the same in tests.
func newEcho(t *testing.T) *echo.Echo {
	t.Helper()
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Validator = validation.NewValidator()
	return e
}

// TestRegisterRequest_ValidatorRejectsShortPassword exercises the validator
// against the auth-handler's registerRequest DTO without spinning up the
// full app.
func TestRegisterRequest_ValidatorRejectsShortPassword(t *testing.T) {
	e := newEcho(t)
	req := registerRequest{
		Name:     "Test",
		Email:    "test@example.com",
		Password: "short",
	}
	// Wire validator like Echo does.
	if err := e.Validator.Validate(&req); err == nil {
		t.Fatal("expected validation failure for short password")
	}
}

// TestRegisterRequest_ValidatorAcceptsValid exercises the happy path.
func TestRegisterRequest_ValidatorAcceptsValid(t *testing.T) {
	e := newEcho(t)
	req := registerRequest{
		Name:     "Test",
		Email:    "test@example.com",
		Password: "password123",
	}
	if err := e.Validator.Validate(&req); err != nil {
		t.Fatalf("unexpected validation failure: %v", err)
	}
}

// TestUpdateStatusRequest_RejectsUnknownStatus shows that the validator
// allow-list catches typos before they reach the use case.
func TestUpdateStatusRequest_RejectsUnknownStatus(t *testing.T) {
	e := newEcho(t)
	req := updateTicketStatusRequest{Status: "bogus"}
	if err := e.Validator.Validate(&req); err == nil {
		t.Fatal("expected validator to reject unknown status")
	}
}

// TestTranslateErr maps the documented sentinel errors to their HTTP codes.
// It runs without a real handler — Echo's response writer is captured via
// httptest.
func TestTranslateErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
	}{
		{"invalid input", domain.ErrInvalidInput, http.StatusUnprocessableEntity},
		{"invalid transition", domain.ErrInvalidStatusTransition, http.StatusUnprocessableEntity},
		{"invalid assignee", domain.ErrInvalidAssignee, http.StatusUnprocessableEntity},
		{"too large", domain.ErrAttachmentTooLarge, http.StatusUnprocessableEntity},
		{"unsupported", domain.ErrAttachmentUnsupported, http.StatusUnprocessableEntity},
		{"category conflict", domain.ErrCategoryConflict, http.StatusConflict},
		{"forbidden", domain.ErrForbidden, http.StatusForbidden},
		{"unauthorized", domain.ErrUnauthorized, http.StatusUnauthorized},
		{"ticket not found", domain.ErrTicketNotFound, http.StatusNotFound},
		{"category not found", domain.ErrTicketCategoryNotFound, http.StatusNotFound},
		{"comment not found", domain.ErrCommentNotFound, http.StatusNotFound},
		{"attachment not found", domain.ErrAttachmentNotFound, http.StatusNotFound},
		{"notification not found", domain.ErrNotificationNotFound, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			c := newEcho(t).NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rr)
			if err := translateErr(c, tc.err); err != nil {
				t.Fatalf("translateErr returned error: %v", err)
			}
			if rr.Code != tc.code {
				t.Errorf("status code: want %d got %d", tc.code, rr.Code)
			}
		})
	}
}

// TestResponseJSONError_Format confirms the standard envelope.
func TestResponseJSONError_Format(t *testing.T) {
	rr := httptest.NewRecorder()
	c := newEcho(t).NewContext(httptest.NewRequest(http.MethodGet, "/", nil), rr)
	if err := response.JSONError(c, http.StatusUnauthorized, "unauthorized"); err != nil {
		t.Fatal(err)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"message":"unauthorized"`) {
		t.Errorf("envelope missing message field: %s", body)
	}
}

// TestParseBoolHelper exercises the notification handler's tiny helper.
func TestParseBoolHelper(t *testing.T) {
	for _, in := range []string{"true", "1", "yes", "TRUE"} {
		if !parseBool(in) {
			t.Errorf("parseBool(%q) = false", in)
		}
	}
	for _, in := range []string{"", "false", "0", "no", "garbage"} {
		if parseBool(in) {
			t.Errorf("parseBool(%q) = true", in)
		}
	}
}
