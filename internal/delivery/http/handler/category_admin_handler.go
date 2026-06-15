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

// CategoryAdminHandler exposes the admin-only category CRUD routes.
type CategoryAdminHandler struct {
	svc *ticket.Service
}

func NewCategoryAdminHandler(svc *ticket.Service) *CategoryAdminHandler {
	return &CategoryAdminHandler{svc: svc}
}

type createCategoryRequest struct {
	Name        string  `json:"name"        validate:"required,min=2,max=100"`
	Slug        string  `json:"slug"        validate:"required,min=2,max=120"`
	Description *string `json:"description" validate:"omitempty,max=2000"`
}

type updateCategoryRequest struct {
	Name        *string `json:"name"        validate:"omitempty,min=2,max=100"`
	Slug        *string `json:"slug"        validate:"omitempty,min=2,max=120"`
	Description *string `json:"description" validate:"omitempty,max=2000"`
	IsActive    *bool   `json:"is_active"`
}

func parseCategoryID(c echo.Context) (uuid.UUID, error) {
	return uuid.Parse(c.Param("category_id"))
}

func (h *CategoryAdminHandler) List(c echo.Context) error {
	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}
	out, err := h.svc.AdminListCategories(ctx, actor)
	if err != nil {
		return translateErr(c, err)
	}
	return response.OK(c, out, "categories retrieved successfully")
}

func (h *CategoryAdminHandler) Create(c echo.Context) error {
	var req createCategoryRequest
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
	out, err := h.svc.AdminCreateCategory(ctx, actor, ticket.CreateCategoryInput{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
	})
	if err != nil {
		return translateErr(c, err)
	}
	return response.Created(c, out, "category created successfully")
}

func (h *CategoryAdminHandler) Update(c echo.Context) error {
	id, err := parseCategoryID(c)
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid category id")
	}
	var req updateCategoryRequest
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
	out, err := h.svc.AdminUpdateCategory(ctx, actor, id, ticket.UpdateCategoryInput{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		IsActive:    req.IsActive,
	})
	if err != nil {
		return translateErr(c, err)
	}
	return response.OK(c, out, "category updated successfully")
}

func (h *CategoryAdminHandler) Deactivate(c echo.Context) error {
	id, err := parseCategoryID(c)
	if err != nil {
		return response.JSONError(c, http.StatusUnprocessableEntity, "invalid category id")
	}
	ctx := c.Request().Context()
	actor, err := h.svc.Actor(ctx, middleware.AuthenticatedUserID(c))
	if err != nil {
		return translateErr(c, err)
	}
	out, err := h.svc.AdminDeactivateCategory(ctx, actor, id)
	if err != nil {
		return translateErr(c, err)
	}
	return response.OK(c, out, "category deactivated successfully")
}
