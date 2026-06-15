package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"ticketing-api/internal/delivery/http/response"
	"ticketing-api/internal/domain"
	"ticketing-api/internal/domain/repository"
	"ticketing-api/internal/domain/service"
)

// Echo-context keys for the authenticated principal.
const (
	CtxAuthenticatedUserID    = "authenticated_user_id"
	CtxAuthenticatedSessionID = "authenticated_session_id"
)

// AuthenticatedUserID returns the user ID stored on the context, or 0 if absent.
func AuthenticatedUserID(c echo.Context) int64 {
	v, _ := c.Get(CtxAuthenticatedUserID).(int64)
	return v
}

// AuthenticatedSessionID returns the session UUID stored on the context.
func AuthenticatedSessionID(c echo.Context) uuid.UUID {
	v, _ := c.Get(CtxAuthenticatedSessionID).(uuid.UUID)
	return v
}

// BearerAuth verifies the Authorization header, the JWT, and the backing
// session, then puts the principal on the Echo context.
func BearerAuth(tokens service.TokenService, sessions repository.AuthSessionRepository) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			header := strings.TrimSpace(c.Request().Header.Get("Authorization"))
			if header == "" {
				return unauthorized(c)
			}
			const prefix = "Bearer "
			if !strings.HasPrefix(header, prefix) {
				return unauthorized(c)
			}
			token := strings.TrimSpace(header[len(prefix):])
			if token == "" {
				return unauthorized(c)
			}

			claims, err := tokens.ParseAccessToken(token)
			if err != nil {
				return unauthorized(c)
			}

			ctx, cancel := context.WithTimeout(c.Request().Context(), defaultLookupTimeout)
			defer cancel()

			session, err := sessions.FindActiveByID(ctx, claims.SessionID)
			if err != nil {
				if errors.Is(err, domain.ErrSessionNotFound) {
					return unauthorized(c)
				}
				return unauthorized(c)
			}
			if session.UserID != claims.UserID {
				return unauthorized(c)
			}

			c.Set(CtxAuthenticatedUserID, claims.UserID)
			c.Set(CtxAuthenticatedSessionID, claims.SessionID)
			return next(c)
		}
	}
}

func unauthorized(c echo.Context) error {
	return response.JSONError(c, http.StatusUnauthorized, "unauthorized")
}
