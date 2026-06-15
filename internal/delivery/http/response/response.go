package response

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// Success is the shape of a successful JSON response.
type Success struct {
	Data    any    `json:"data"`
	Message string `json:"message,omitempty"`
}

// PaginatedSuccess wraps a list response with pagination metadata.
type PaginatedSuccess struct {
	Data    any    `json:"data"`
	Message string `json:"message,omitempty"`
	Meta    Meta   `json:"meta"`
}

// Meta carries pagination information for list endpoints.
type Meta struct {
	Page        int    `json:"page"`
	PerPage     int    `json:"per_page"`
	Total       int64  `json:"total"`
	TotalPages  int    `json:"total_pages"`
	UnreadTotal *int64 `json:"unread_total,omitempty"`
}

// Error is the shape of a failure JSON response.
type Error struct {
	Message string              `json:"message"`
	Errors  map[string][]string `json:"errors,omitempty"`
}

// OK writes 200 with data and optional message.
func OK(c echo.Context, data any, message string) error {
	return c.JSON(http.StatusOK, Success{Data: data, Message: message})
}

// Created writes 201 with data and optional message.
func Created(c echo.Context, data any, message string) error {
	return c.JSON(http.StatusCreated, Success{Data: data, Message: message})
}

// Paginated writes 200 with data, optional message, and pagination meta.
func Paginated(c echo.Context, data any, message string, meta Meta) error {
	return c.JSON(http.StatusOK, PaginatedSuccess{Data: data, Message: message, Meta: meta})
}

// NoContent writes 204 with an empty body.
func NoContent(c echo.Context) error {
	return c.NoContent(http.StatusNoContent)
}

// JSONError writes a non-validation failure response.
func JSONError(c echo.Context, status int, message string) error {
	return c.JSON(status, Error{Message: message})
}

// ValidationError writes a 422 validation failure response.
func ValidationError(c echo.Context, message string, fields map[string][]string) error {
	return c.JSON(http.StatusUnprocessableEntity, Error{Message: message, Errors: fields})
}
