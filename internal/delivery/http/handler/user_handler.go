package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"ticketing-api/internal/delivery/http/middleware"
	"ticketing-api/internal/delivery/http/response"
	"ticketing-api/internal/delivery/http/validation"
	"ticketing-api/internal/domain"
	"ticketing-api/internal/usecase/user"
)

type UserHandler struct {
	svc *user.Service
}

func NewUserHandler(svc *user.Service) *UserHandler {
	return &UserHandler{svc: svc}
}

type updateProfileRequest struct {
	Name string `json:"name" validate:"required,min=2,max=100"`
}

func (h *UserHandler) Me(c echo.Context) error {
	uid := middleware.AuthenticatedUserID(c)
	profile, err := h.svc.GetProfile(c.Request().Context(), uid)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return response.JSONError(c, http.StatusUnauthorized, "unauthorized")
		}
		return response.JSONError(c, http.StatusInternalServerError, "internal server error")
	}
	return response.OK(c, profile, "")
}

func (h *UserHandler) UpdateMe(c echo.Context) error {
	uid := middleware.AuthenticatedUserID(c)

	var req updateProfileRequest
	if err := c.Bind(&req); err != nil {
		return response.JSONError(c, http.StatusBadRequest, "invalid request body")
	}
	if err := c.Validate(&req); err != nil {
		return response.ValidationError(c, "validation failed", validation.FormatErrors(err))
	}

	profile, err := h.svc.UpdateProfile(c.Request().Context(), uid, user.UpdateProfileInput{Name: req.Name})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidInput):
			return response.JSONError(c, http.StatusUnprocessableEntity, "validation failed")
		case errors.Is(err, domain.ErrUserNotFound):
			return response.JSONError(c, http.StatusUnauthorized, "unauthorized")
		}
		return response.JSONError(c, http.StatusInternalServerError, "internal server error")
	}
	return response.OK(c, profile, "profile updated successfully")
}
