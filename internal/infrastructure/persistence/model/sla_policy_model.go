package model

import (
	"time"

	"github.com/google/uuid"

	"ticketing-api/internal/domain/entity"
)

type SLAPolicyModel struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey;column:id"`
	Priority          string    `gorm:"column:priority;size:20;not null;uniqueIndex"`
	ResponseMinutes   int       `gorm:"column:response_minutes;not null"`
	ResolutionMinutes int       `gorm:"column:resolution_minutes;not null"`
	IsActive          bool      `gorm:"column:is_active;not null;default:true"`
	CreatedAt         time.Time `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt         time.Time `gorm:"column:updated_at;not null;autoUpdateTime"`
}

func (SLAPolicyModel) TableName() string { return "sla_policies" }

func (m *SLAPolicyModel) ToEntity() *entity.SLAPolicy {
	return &entity.SLAPolicy{
		ID:                m.ID,
		Priority:          m.Priority,
		ResponseMinutes:   m.ResponseMinutes,
		ResolutionMinutes: m.ResolutionMinutes,
		IsActive:          m.IsActive,
		CreatedAt:         m.CreatedAt,
		UpdatedAt:         m.UpdatedAt,
	}
}
