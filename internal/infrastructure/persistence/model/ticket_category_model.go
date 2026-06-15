package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ticketing-api/internal/domain/entity"
)

// TicketCategoryModel is the GORM-bound row for ticket_categories.
type TicketCategoryModel struct {
	ID          uuid.UUID      `gorm:"type:uuid;primaryKey;column:id"`
	Name        string         `gorm:"column:name;size:100;not null"`
	Slug        string         `gorm:"column:slug;size:120;not null;uniqueIndex"`
	Description *string        `gorm:"column:description"`
	IsActive    bool           `gorm:"column:is_active;not null;default:true"`
	CreatedAt   time.Time      `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt   time.Time      `gorm:"column:updated_at;not null;autoUpdateTime"`
	DeletedAt   gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

func (TicketCategoryModel) TableName() string { return "ticket_categories" }

func (m *TicketCategoryModel) ToEntity() *entity.TicketCategory {
	out := &entity.TicketCategory{
		ID:          m.ID,
		Name:        m.Name,
		Slug:        m.Slug,
		Description: m.Description,
		IsActive:    m.IsActive,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
	if m.DeletedAt.Valid {
		t := m.DeletedAt.Time
		out.DeletedAt = &t
	}
	return out
}
