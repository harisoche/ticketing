package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"ticketing-api/internal/delivery/http/middleware"
	"ticketing-api/internal/delivery/http/response"
	"ticketing-api/internal/delivery/http/validation"
	"ticketing-api/internal/domain"
	"ticketing-api/internal/usecase/ticket"
)

type TicketHandler struct {
	svc *ticket.Service
}

func NewTicketHandler(svc *ticket.Service) *TicketHandler {
	return &TicketHandler{svc: svc}
}

// ----- Request DTOs -----

type createTicketRequest struct {
	Title       string    `json:"title"       validate:"required,min=5,max=255"`
	Description string    `json:"description" validate:"required,min=10"`
	CategoryID  uuid.UUID `json:"category_id" validate:"required"`
	Priority    string    `json:"priority"    validate:"omitempty,oneof=low medium high urgent"`
}

type updateTicketRequest struct {
	Title       *string    `json:"title"       validate:"omitempty,min=5,max=255"`
	Description *string    `json:"description" validate:"omitempty,min=10"`
	CategoryID  *uuid.UUID `json:"category_id" validate:"omitempty"`
	Priority    *string    `json:"priority"    validate:"omitempty,oneof=low medium high urgent"`
}

type updateTicketStatusRequest struct {
	Status string  `json:"status" validate:"required,oneof=open in_progress resolved closed reopened"`
	Note   *string `json:"note"   validate:"omitempty,max=500"`
}

// assignTicketRequest is the Phase 3 assignment payload.
// agent_id is required (admins assign / reassign; unassign is not exposed).
type assignTicketRequest struct {
	AgentID *int64  `json:"agent_id" validate:"required"`
	Note    *string `json:"note"     validate:"omitempty,max=500"`
}

// ----- Helpers -----

func parseTicketID(c echo.Context) (uuid.UUID, error) {
	return uuid.Parse(c.Param("id"))
}

func optionalInt64Query(c echo.Context, key string) (*int64, error) {
	raw := strings.TrimSpace(c.QueryParam(key))
	if raw == "" {
		return nil, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func optionalUUIDQuery(c echo.Context, key string) (*uuid.UUID, error) {
	raw := strings.TrimSpace(c.QueryParam(key))
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func optionalIntQuery(c echo.Context, key string, def int) int {
	raw := strings.TrimSpace(c.QueryParam(key))
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < 1 {
		return def
	}
	return v
}

// optionalIntQueryRaw returns the parsed integer or 0 when absent. Errors
// are surfaced as negative values so the use case can reject them with 422.
// Phase 6 wants invalid page / per_page to fail loudly rather than coerce.
func optionalIntQueryRaw(c echo.Context, key string) int {
	raw := strings.TrimSpace(c.QueryParam(key))
	if raw == "" {
		return 0
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return -1
	}
	return v
}

// optionalTimeQuery parses an RFC3339 timestamp. Empty input is OK and
// returns nil.
func optionalTimeQuery(c echo.Context, key string) (*time.Time, error) {
	raw := strings.TrimSpace(c.QueryParam(key))
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// translateErr maps domain errors to HTTP responses. Anything unhandled
// becomes a 500.
func translateErr(c echo.Context, err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidInput),
		errors.Is(err, domain.ErrInvalidStatusTransition),
		errors.Is(err, domain.ErrInvalidAssignee),
		errors.Is(err, domain.ErrTicketNotAssigned),
		errors.Is(err, domain.ErrAttachmentTooLarge),
		errors.Is(err, domain.ErrAttachmentUnsupported):
		return response.JSONError(c, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, domain.ErrCategoryConflict):
		return response.JSONError(c, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrAttachmentNotFound):
		return response.JSONError(c, http.StatusNotFound, "attachment not found")
	case errors.Is(err, domain.ErrNotificationNotFound):
		return response.JSONError(c, http.StatusNotFound, "notification not found")
	case errors.Is(err, domain.ErrSLAPolicyNotFound):
		return response.JSONError(c, http.StatusInternalServerError, "missing sla policy")
	case errors.Is(err, domain.ErrTicketNotFound),
		errors.Is(err, domain.ErrTicketCategoryNotFound):
		return response.JSONError(c, http.StatusNotFound, "ticket not found")
	case errors.Is(err, domain.ErrCommentNotFound):
		return response.JSONError(c, http.StatusNotFound, "comment not found")
	case errors.Is(err, domain.ErrForbidden):
		return response.JSONError(c, http.StatusForbidden, "forbidden")
	case errors.Is(err, domain.ErrUnauthorized):
		return response.JSONError(c, http.StatusUnauthorized, "unauthorized")
	}
	return response.JSONError(c, http.StatusInternalServerError, "internal server error")
}

// ----- Handlers -----

func (h *TicketHandler) Create(c echo.Context) error {
	var req createTicketRequest
	if err := c.Bind(&req); err != nil {
		return response.JSONError(c, http.StatusBadRequest, "invalid request body")
	}
	if err := c.Validate(&req); err != nil {
		return response.ValidationError(c, "validation failed", validation.FormatErrors(err))
	}

	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}

	out, err := h.svc.Create(ctx, actor, ticket.CreateTicketInput{
		Title:       req.Title,
		Description: req.Description,
		CategoryID:  req.CategoryID,
		Priority:    req.Priority,
	})
	if err != nil {
		return translateErr(c, err)
	}
	return response.Created(c, out, "ticket created successfully")
}

func (h *TicketHandler) List(c echo.Context) error {
	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}

	categoryID, err := optionalUUIDQuery(c, "category_id")
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid category_id")
	}
	createdBy, err := optionalInt64Query(c, "created_by")
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid created_by")
	}
	assignedTo, err := optionalInt64Query(c, "assigned_to")
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid assigned_to")
	}

	createdFrom, err := optionalTimeQuery(c, "created_from")
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid created_from")
	}
	createdTo, err := optionalTimeQuery(c, "created_to")
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid created_to")
	}

	in := ticket.ListInput{
		Page:        optionalIntQueryRaw(c, "page"),
		PerPage:     optionalIntQueryRaw(c, "per_page"),
		Search:      strings.TrimSpace(c.QueryParam("search")),
		Query:       strings.TrimSpace(c.QueryParam("q")),
		Status:      strings.TrimSpace(c.QueryParam("status")),
		Priority:    strings.TrimSpace(c.QueryParam("priority")),
		CategoryID:  categoryID,
		CreatedBy:   createdBy,
		AssignedTo:  assignedTo,
		CreatedFrom: createdFrom,
		CreatedTo:   createdTo,
		Scope:       strings.TrimSpace(c.QueryParam("scope")),
		View:        strings.TrimSpace(c.QueryParam("view")),
		SortBy:      strings.TrimSpace(c.QueryParam("sort_by")),
		SortOrder:   strings.TrimSpace(c.QueryParam("sort_order")),
	}

	out, err := h.svc.List(ctx, actor, in)
	if err != nil {
		return translateErr(c, err)
	}
	return response.Paginated(c, out.Items, "tickets retrieved successfully", response.Meta{
		Page:       out.Page,
		PerPage:    out.PerPage,
		Total:      out.Total,
		TotalPages: out.TotalPages,
	})
}

func (h *TicketHandler) Get(c echo.Context) error {
	id, err := parseTicketID(c)
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid ticket id")
	}
	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}

	out, err := h.svc.Get(ctx, actor, id)
	if err != nil {
		return translateErr(c, err)
	}
	return response.OK(c, out, "ticket retrieved successfully")
}

func (h *TicketHandler) Update(c echo.Context) error {
	id, err := parseTicketID(c)
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid ticket id")
	}

	var req updateTicketRequest
	if err := c.Bind(&req); err != nil {
		return response.JSONError(c, http.StatusBadRequest, "invalid request body")
	}
	if err := c.Validate(&req); err != nil {
		return response.ValidationError(c, "validation failed", validation.FormatErrors(err))
	}

	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}

	out, err := h.svc.Update(ctx, actor, id, ticket.UpdateTicketInput{
		Title:       req.Title,
		Description: req.Description,
		CategoryID:  req.CategoryID,
		Priority:    req.Priority,
	})
	if err != nil {
		return translateErr(c, err)
	}
	return response.OK(c, out, "ticket updated successfully")
}

func (h *TicketHandler) UpdateStatus(c echo.Context) error {
	id, err := parseTicketID(c)
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid ticket id")
	}

	var req updateTicketStatusRequest
	if err := c.Bind(&req); err != nil {
		return response.JSONError(c, http.StatusBadRequest, "invalid request body")
	}
	if err := c.Validate(&req); err != nil {
		return response.ValidationError(c, "validation failed", validation.FormatErrors(err))
	}

	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}

	out, err := h.svc.UpdateStatus(ctx, actor, id, ticket.UpdateStatusInput{Status: req.Status, Note: req.Note})
	if err != nil {
		return translateErr(c, err)
	}
	return response.OK(c, out, "ticket status updated successfully")
}

func (h *TicketHandler) Assign(c echo.Context) error {
	id, err := parseTicketID(c)
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid ticket id")
	}

	var req assignTicketRequest
	if err := c.Bind(&req); err != nil {
		return response.JSONError(c, http.StatusBadRequest, "invalid request body")
	}
	if err := c.Validate(&req); err != nil {
		return response.ValidationError(c, "validation failed", validation.FormatErrors(err))
	}

	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}

	out, err := h.svc.Assign(ctx, actor, id, ticket.AssignInput{AgentID: req.AgentID, Note: req.Note})
	if err != nil {
		return translateErr(c, err)
	}
	return response.OK(c, out, "ticket assigned successfully")
}

// Histories handles GET /api/v1/tickets/:id/histories.
func (h *TicketHandler) Histories(c echo.Context) error {
	id, err := parseTicketID(c)
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid ticket id")
	}
	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}
	rows, err := h.svc.ListHistories(ctx, actor, id)
	if err != nil {
		return translateErr(c, err)
	}
	return response.OK(c, rows, "ticket histories retrieved successfully")
}

func (h *TicketHandler) Delete(c echo.Context) error {
	id, err := parseTicketID(c)
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid ticket id")
	}
	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}
	if err := h.svc.Delete(ctx, actor, id); err != nil {
		return translateErr(c, err)
	}
	return response.NoContent(c)
}
