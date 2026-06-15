package handler

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"ticketing-api/internal/delivery/http/middleware"
	"ticketing-api/internal/delivery/http/response"
	"ticketing-api/internal/delivery/http/validation"
	"ticketing-api/internal/domain"
	"ticketing-api/internal/usecase/auth"
)

type AuthHandler struct {
	svc *auth.Service
}

func NewAuthHandler(svc *auth.Service) *AuthHandler {
	return &AuthHandler{svc: svc}
}

type registerRequest struct {
	Name     string `json:"name"     validate:"required,min=2,max=100"`
	Email    string `json:"email"    validate:"required,email,max=255"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

type loginRequest struct {
	Email    string `json:"email"    validate:"required,email,max=255"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

func (h *AuthHandler) Register(c echo.Context) error {
	var req registerRequest
	if err := c.Bind(&req); err != nil {
		return response.JSONError(c, http.StatusBadRequest, "invalid request body")
	}
	if err := c.Validate(&req); err != nil {
		return response.ValidationError(c, "validation failed", validation.FormatErrors(err))
	}

	res, err := h.svc.Register(c.Request().Context(), auth.RegisterInput{
		Name:     req.Name,
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrEmailAlreadyExists):
			return response.JSONError(c, http.StatusConflict, "email already registered")
		case errors.Is(err, domain.ErrInvalidInput):
			return response.JSONError(c, http.StatusUnprocessableEntity, "validation failed")
		}
		return response.JSONError(c, http.StatusInternalServerError, "internal server error")
	}
	return response.Created(c, res, "registration successful")
}

func (h *AuthHandler) Login(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil {
		return response.JSONError(c, http.StatusBadRequest, "invalid request body")
	}
	if err := c.Validate(&req); err != nil {
		return response.ValidationError(c, "validation failed", validation.FormatErrors(err))
	}

	res, err := h.svc.Login(c.Request().Context(), auth.LoginInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			return response.JSONError(c, http.StatusUnauthorized, "invalid email or password")
		}
		return response.JSONError(c, http.StatusInternalServerError, "internal server error")
	}
	return response.OK(c, res, "login successful")
}

func (h *AuthHandler) Logout(c echo.Context) error {
	sid := middleware.AuthenticatedSessionID(c)
	if err := h.svc.Logout(c.Request().Context(), sid); err != nil {
		return response.JSONError(c, http.StatusInternalServerError, "internal server error")
	}
	return response.OK(c, nil, "logout successful")
}
