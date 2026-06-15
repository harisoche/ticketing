package handler

import (
	"github.com/labstack/echo/v4"

	"ticketing-api/internal/delivery/http/response"
)

type HealthHandler struct{}

func NewHealthHandler() *HealthHandler { return &HealthHandler{} }

func (h *HealthHandler) Get(c echo.Context) error {
	return response.OK(c, map[string]string{"status": "ok"}, "")
}
