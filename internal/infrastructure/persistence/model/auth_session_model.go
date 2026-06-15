package model

import (
	"time"

	"github.com/google/uuid"

	"ticketing-api/internal/domain/entity"
)

// AuthSessionModel is the GORM-bound representation of a row in the
// `auth_sessions` table.
type AuthSessionModel struct {
	ID        uuid.UUID  `gorm:"primaryKey;type:uuid;column:id"`
	UserID    int64      `gorm:"column:user_id;not null"`
	ExpiresAt time.Time  `gorm:"column:expires_at;not null"`
	RevokedAt *time.Time `gorm:"column:revoked_at"`
	CreatedAt time.Time  `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt time.Time  `gorm:"column:updated_at;not null;autoUpdateTime"`
}

func (AuthSessionModel) TableName() string { return "auth_sessions" }

func AuthSessionModelFromEntity(s *entity.AuthSession) *AuthSessionModel {
	return &AuthSessionModel{
		ID:        s.ID,
		UserID:    s.UserID,
		ExpiresAt: s.ExpiresAt,
		RevokedAt: s.RevokedAt,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
}

func (m *AuthSessionModel) ToEntity() *entity.AuthSession {
	return &entity.AuthSession{
		ID:        m.ID,
		UserID:    m.UserID,
		ExpiresAt: m.ExpiresAt,
		RevokedAt: m.RevokedAt,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}
