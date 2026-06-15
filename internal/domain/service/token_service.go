package service

import (
	"time"

	"github.com/google/uuid"
)

// TokenClaims is the framework-independent shape of an access-token payload.
type TokenClaims struct {
	UserID    int64
	SessionID uuid.UUID
	ExpiresAt time.Time
}

// TokenService generates and parses bearer access tokens.
type TokenService interface {
	GenerateAccessToken(userID int64, sessionID uuid.UUID, expiresAt time.Time) (string, error)
	ParseAccessToken(token string) (*TokenClaims, error)
}
