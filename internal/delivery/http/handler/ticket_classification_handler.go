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

// Reuses TicketHandler.svc via composition would be tidy but we keep handlers
// flat for the educational project. This handler is a thin shim on the same
// ticket.Service.
type TicketClassificationHandler struct {
	svc *ticket.Service
}

func NewTicketClassificationHandler(svc *ticket.Service) *TicketClassificationHandler {
	return &TicketClassificationHandler{svc: svc}
}

type classifyRequest struct {
	CategoryID *uuid.UUID `json:"category_id" validate:"omitempty"`
	Priority   *string    `json:"priority"    validate:"omitempty,oneof=low medium high urgent"`
}

func (h *TicketClassificationHandler) Update(c echo.Context) error {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid ticket id")
	}
	var req classifyRequest
	if err := c.Bind(&req); err != nil {
		return response.JSONError(c, http.StatusBadRequest, "invalid request body")
	}
	if err := c.Validate(&req); err != nil {
		return response.ValidationError(c, "validation failed", validation.FormatErrors(err))
	}
	if req.CategoryID == nil && req.Priority == nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "category_id or priority is required")
	}
	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}
	out, err := h.svc.ClassifyTicket(ctx, actor, id, ticket.ClassifyTicketInput{
		CategoryID: req.CategoryID,
		Priority:   req.Priority,
	})
	if err != nil {
		return translateErr(c, err)
	}
	return response.OK(c, out, "ticket classification updated successfully")
}
