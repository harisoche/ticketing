package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"ticketing-api/internal/delivery/http/response"
	"ticketing-api/internal/usecase/ticket"
)

type TicketCategoryHandler struct {
	svc *ticket.Service
}

func NewTicketCategoryHandler(svc *ticket.Service) *TicketCategoryHandler {
	return &TicketCategoryHandler{svc: svc}
}

func (h *TicketCategoryHandler) List(c echo.Context) error {
	cats, err := h.svc.ListActiveCategories(c.Request().Context())
	if err != nil {
		return response.JSONError(c, http.StatusInternalServerError, "internal server error")
	}
	return response.OK(c, cats, "ticket categories retrieved successfully")
}
