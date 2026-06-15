package handler

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"ticketing-api/internal/delivery/http/middleware"
	"ticketing-api/internal/delivery/http/response"
	"ticketing-api/internal/delivery/http/validation"
	"ticketing-api/internal/usecase/ticket"
)

// TicketCommentHandler bundles the Phase 4 routes for comments and timeline.
// It shares the underlying ticket.Service with TicketHandler so it reuses
// the same canViewTicket access helper.
type TicketCommentHandler struct {
	svc *ticket.Service
}

func NewTicketCommentHandler(svc *ticket.Service) *TicketCommentHandler {
	return &TicketCommentHandler{svc: svc}
}

type commentRequest struct {
	Body string `json:"body" validate:"required,min=1,max=5000"`
}

func parseTicketParam(c echo.Context) (uuid.UUID, error) {
	return uuid.Parse(c.Param("id"))
}

func parseCommentParam(c echo.Context) (uuid.UUID, error) {
	return uuid.Parse(c.Param("comment_id"))
}

func (h *TicketCommentHandler) Create(c echo.Context) error {
	ticketID, err := parseTicketParam(c)
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid ticket id")
	}
	var req commentRequest
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
	out, err := h.svc.AddComment(ctx, actor, ticketID, ticket.AddCommentInput{Body: req.Body})
	if err != nil {
		return translateErr(c, err)
	}
	return response.Created(c, out, "comment created successfully")
}

func (h *TicketCommentHandler) List(c echo.Context) error {
	ticketID, err := parseTicketParam(c)
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid ticket id")
	}
	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}
	out, err := h.svc.ListComments(ctx, actor, ticketID)
	if err != nil {
		return translateErr(c, err)
	}
	return response.OK(c, out, "comments retrieved successfully")
}

func (h *TicketCommentHandler) Update(c echo.Context) error {
	ticketID, err := parseTicketParam(c)
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid ticket id")
	}
	commentID, err := parseCommentParam(c)
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid comment id")
	}
	var req commentRequest
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
	out, err := h.svc.UpdateComment(ctx, actor, ticketID, commentID, ticket.UpdateCommentInput{Body: req.Body})
	if err != nil {
		return translateErr(c, err)
	}
	return response.OK(c, out, "comment updated successfully")
}

func (h *TicketCommentHandler) Delete(c echo.Context) error {
	ticketID, err := parseTicketParam(c)
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid ticket id")
	}
	commentID, err := parseCommentParam(c)
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid comment id")
	}
	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}
	if err := h.svc.DeleteComment(ctx, actor, ticketID, commentID); err != nil {
		return translateErr(c, err)
	}
	return response.NoContent(c)
}

func (h *TicketCommentHandler) Timeline(c echo.Context) error {
	ticketID, err := parseTicketParam(c)
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid ticket id")
	}
	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}
	out, err := h.svc.GetTimeline(ctx, actor, ticketID)
	if err != nil {
		return translateErr(c, err)
	}
	return response.OK(c, out, "ticket timeline retrieved successfully")
}
