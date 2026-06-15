package handler

import (
	"github.com/labstack/echo/v4"

	"ticketing-api/internal/delivery/http/middleware"
	"ticketing-api/internal/delivery/http/response"
	"ticketing-api/internal/usecase/ticket"
)

// DashboardHandler exposes the role-scoped summary endpoint.
type DashboardHandler struct {
	svc *ticket.Service
}

func NewDashboardHandler(svc *ticket.Service) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

func (h *DashboardHandler) Summary(c echo.Context) error {
	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}
	out, err := h.svc.DashboardSummary(ctx, actor)
	if err != nil {
		return translateErr(c, err)
	}
	return response.OK(c, out, "dashboard summary retrieved successfully")
}
