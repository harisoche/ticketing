package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"ticketing-api/internal/delivery/http/middleware"
	"ticketing-api/internal/delivery/http/response"
	"ticketing-api/internal/usecase/ticket"
)

type NotificationHandler struct {
	svc *ticket.Service
}

func NewNotificationHandler(svc *ticket.Service) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

func parseBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes":
		return true
	}
	return false
}

func (h *NotificationHandler) List(c echo.Context) error {
	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}

	page, err := strconv.Atoi(strings.TrimSpace(c.QueryParam("page")))
	if err != nil && c.QueryParam("page") != "" {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid page")
	}
	per, err := strconv.Atoi(strings.TrimSpace(c.QueryParam("per_page")))
	if err != nil && c.QueryParam("per_page") != "" {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid per_page")
	}

	out, err := h.svc.ListNotifications(ctx, actor, ticket.NotificationListInput{
		Page: page, PerPage: per, UnreadOnly: parseBool(c.QueryParam("unread_only")),
	})
	if err != nil {
		return translateErr(c, err)
	}
	return response.Paginated(c, out.Items, "notifications retrieved successfully", response.Meta{
		Page: out.Page, PerPage: out.PerPage, Total: out.Total, TotalPages: out.TotalPages,
		UnreadTotal: &out.UnreadTotal,
	})
}

func (h *NotificationHandler) MarkRead(c echo.Context) error {
	id, err := uuid.Parse(c.Param("notification_id"))
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid notification id")
	}
	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}
	if err := h.svc.MarkNotificationRead(ctx, actor, id); err != nil {
		return translateErr(c, err)
	}
	return response.OK(c, nil, "notification marked as read")
}

func (h *NotificationHandler) MarkAllRead(c echo.Context) error {
	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}
	count, err := h.svc.MarkAllNotificationsRead(ctx, actor)
	if err != nil {
		return translateErr(c, err)
	}
	return response.OK(c, map[string]int64{"marked_read": count}, "all notifications marked as read")
}
