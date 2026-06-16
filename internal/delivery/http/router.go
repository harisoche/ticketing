package http

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"

	"ticketing-api/internal/delivery/http/handler"
	"ticketing-api/internal/delivery/http/middleware"
	"ticketing-api/internal/delivery/http/response"
	"ticketing-api/internal/delivery/http/validation"
	"ticketing-api/internal/domain/repository"
	"ticketing-api/internal/domain/service"
)

type Handlers struct {
	Health           *handler.HealthHandler
	Auth             *handler.AuthHandler
	User             *handler.UserHandler
	Ticket           *handler.TicketHandler
	TicketCategory   *handler.TicketCategoryHandler
	TicketComment    *handler.TicketCommentHandler
	CategoryAdmin    *handler.CategoryAdminHandler
	Classification   *handler.TicketClassificationHandler
	TicketAttachment *handler.TicketAttachmentHandler
	Dashboard        *handler.DashboardHandler
	Notification     *handler.NotificationHandler
	Docs             *handler.DocsHandler
}

type RouterDeps struct {
	Tokens   service.TokenService
	Sessions repository.AuthSessionRepository
}

// NewEcho constructs an Echo instance with logging, recovery, the custom
// validator, and a central JSON error handler.
func NewEcho() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	e.Validator = validation.NewValidator()
	e.HTTPErrorHandler = jsonErrorHandler

	e.Use(echomw.RequestID())
	e.Use(echomw.Logger())
	e.Use(echomw.Recover())

	return e
}

// RegisterRoutes wires the route table onto the Echo instance.
func RegisterRoutes(e *echo.Echo, h Handlers, deps RouterDeps) {
	e.GET("/health", h.Health.Get)

	if h.Docs != nil {
		e.GET("/docs", h.Docs.UI)
		e.GET("/docs/openapi.yaml", h.Docs.Spec)
	}

	api := e.Group("/api/v1")
	api.POST("/auth/register", h.Auth.Register)
	api.POST("/auth/login", h.Auth.Login)

	bearer := middleware.BearerAuth(deps.Tokens, deps.Sessions)
	protected := api.Group("", bearer)
	protected.POST("/auth/logout", h.Auth.Logout)
	protected.GET("/me", h.User.Me)
	protected.PATCH("/me", h.User.UpdateMe)

	protected.GET("/ticket-categories", h.TicketCategory.List)
	protected.POST("/tickets", h.Ticket.Create)
	protected.GET("/tickets", h.Ticket.List)
	protected.GET("/tickets/:id", h.Ticket.Get)
	protected.PATCH("/tickets/:id", h.Ticket.Update)
	protected.PATCH("/tickets/:id/status", h.Ticket.UpdateStatus)
	protected.PATCH("/tickets/:id/assign", h.Ticket.Assign)
	protected.GET("/tickets/:id/histories", h.Ticket.Histories)
	protected.DELETE("/tickets/:id", h.Ticket.Delete)

	protected.POST("/tickets/:id/comments", h.TicketComment.Create)
	protected.GET("/tickets/:id/comments", h.TicketComment.List)
	protected.PUT("/tickets/:id/comments/:comment_id", h.TicketComment.Update)
	protected.DELETE("/tickets/:id/comments/:comment_id", h.TicketComment.Delete)
	protected.GET("/tickets/:id/timeline", h.TicketComment.Timeline)

	// Phase 5
	protected.GET("/categories", h.TicketCategory.List)
	protected.GET("/admin/categories", h.CategoryAdmin.List)
	protected.POST("/admin/categories", h.CategoryAdmin.Create)
	protected.PUT("/admin/categories/:category_id", h.CategoryAdmin.Update)
	protected.DELETE("/admin/categories/:category_id", h.CategoryAdmin.Deactivate)

	protected.PUT("/tickets/:id/classification", h.Classification.Update)

	protected.POST("/tickets/:id/attachments", h.TicketAttachment.Upload)
	protected.GET("/tickets/:id/attachments", h.TicketAttachment.List)
	protected.GET("/tickets/:id/attachments/:attachment_id/download", h.TicketAttachment.Download)
	protected.DELETE("/tickets/:id/attachments/:attachment_id", h.TicketAttachment.Delete)

	// Phase 6
	protected.GET("/dashboard/summary", h.Dashboard.Summary)

	// Phase 7
	protected.GET("/notifications", h.Notification.List)
	protected.PUT("/notifications/read-all", h.Notification.MarkAllRead)
	protected.PUT("/notifications/:notification_id/read", h.Notification.MarkRead)
}

// jsonErrorHandler converts Echo errors (404/405/etc) and panics into the
// project's standard JSON error shape.
func jsonErrorHandler(err error, c echo.Context) {
	if c.Response().Committed {
		return
	}

	status := http.StatusInternalServerError
	message := "internal server error"

	var he *echo.HTTPError
	if errors.As(err, &he) {
		status = he.Code
		switch m := he.Message.(type) {
		case string:
			message = m
		case error:
			message = m.Error()
		default:
			message = http.StatusText(status)
		}
	}

	_ = response.JSONError(c, status, message)
}
