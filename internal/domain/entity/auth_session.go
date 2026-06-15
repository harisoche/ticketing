package entity

import (
	"time"

	"github.com/google/uuid"
)

// AuthSession represents a single bearer-token session for a user. It backs
// JWT validation: the JWT's `jti` claim equals this entity's ID.
type AuthSession struct {
	ID        uuid.UUID
	UserID    int64
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsActive reports whether the session is currently valid: not revoked and
// not past its expiration. `now` is supplied so the caller controls the
// time source (use UTC).
func (s *AuthSession) IsActive(now time.Time) bool {
	if s.RevokedAt != nil {
		return false
	}
	return s.ExpiresAt.After(now)
}
