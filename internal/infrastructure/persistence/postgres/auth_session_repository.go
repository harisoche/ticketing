package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ticketing-api/internal/domain"
	"ticketing-api/internal/domain/entity"
	"ticketing-api/internal/domain/repository"
	"ticketing-api/internal/infrastructure/persistence/model"
)

type authSessionRepository struct {
	db *gorm.DB
}

func NewAuthSessionRepository(db *gorm.DB) repository.AuthSessionRepository {
	return &authSessionRepository{db: db}
}

func (r *authSessionRepository) Create(ctx context.Context, session *entity.AuthSession) error {
	m := model.AuthSessionModelFromEntity(session)
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		return err
	}
	session.CreatedAt = m.CreatedAt
	session.UpdatedAt = m.UpdatedAt
	return nil
}

// FindActiveByID returns the session ONLY if it is unrevoked and unexpired.
// Database-level filtering ensures revoked/expired sessions are never returned.
func (r *authSessionRepository) FindActiveByID(ctx context.Context, id uuid.UUID) (*entity.AuthSession, error) {
	var m model.AuthSessionModel
	err := r.db.WithContext(ctx).
		Where("id = ? AND revoked_at IS NULL AND expires_at > NOW()", id).
		First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrSessionNotFound
		}
		return nil, err
	}
	return m.ToEntity(), nil
}

// Revoke is idempotent: revoking an already-revoked session is not an error.
func (r *authSessionRepository) Revoke(ctx context.Context, id uuid.UUID, revokedAt time.Time) error {
	tx := r.db.WithContext(ctx).Model(&model.AuthSessionModel{}).
		Where("id = ? AND revoked_at IS NULL", id).
		Update("revoked_at", revokedAt)
	if tx.Error != nil {
		return tx.Error
	}
	// RowsAffected may be 0 if already revoked — that's fine.
	return nil
}
