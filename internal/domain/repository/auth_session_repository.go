package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"ticketing-api/internal/domain/entity"
)

// AuthSessionRepository persists and retrieves bearer-token sessions.
type AuthSessionRepository interface {
	Create(ctx context.Context, session *entity.AuthSession) error
	FindActiveByID(ctx context.Context, id uuid.UUID) (*entity.AuthSession, error)
	Revoke(ctx context.Context, id uuid.UUID, revokedAt time.Time) error
}
