package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ticketing-api/internal/domain/entity"
)

// TicketModel is the GORM-bound row for tickets.
//
// Note: created_by / assigned_to are int64 in this project because
// users.id is BIGINT IDENTITY (see Phase 1 schema).
type TicketModel struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;column:id"`
	Code        string     `gorm:"column:code;size:20;not null;uniqueIndex;<-:false"`
	Title       string     `gorm:"column:title;size:255;not null"`
	Description string     `gorm:"column:description;type:text;not null"`
	Status      string     `gorm:"column:status;size:30;not null"`
	Priority    string     `gorm:"column:priority;size:20;not null"`
	CategoryID  uuid.UUID  `gorm:"column:category_id;type:uuid;not null"`
	CreatedBy   int64      `gorm:"column:created_by;not null"`
	AssignedTo  *int64     `gorm:"column:assigned_to"`
	AssignedAt  *time.Time `gorm:"column:assigned_at"`

	ResponseDueAt    *time.Time `gorm:"column:response_due_at"`
	ResolutionDueAt  *time.Time `gorm:"column:resolution_due_at"`
	FirstRespondedAt *time.Time `gorm:"column:first_responded_at"`
	ResolvedAt       *time.Time `gorm:"column:resolved_at"`
	ClosedAt         *time.Time `gorm:"column:closed_at"`

	CreatedAt time.Time      `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt time.Time      `gorm:"column:updated_at;not null;autoUpdateTime"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index"`

	Category *TicketCategoryModel `gorm:"foreignKey:CategoryID;references:ID"`
	Creator  *UserModel           `gorm:"foreignKey:CreatedBy;references:ID"`
	Assignee *UserModel           `gorm:"foreignKey:AssignedTo;references:ID"`
}

func (TicketModel) TableName() string { return "tickets" }

// FromEntity copies the writable fields from an entity into the model. Code
// is omitted — the database default generates it via the ticket_code_seq
// sequence on INSERT.
func TicketModelFromEntity(t *entity.Ticket) *TicketModel {
	m := &TicketModel{
		ID:               t.ID,
		Title:            t.Title,
		Description:      t.Description,
		Status:           t.Status,
		Priority:         t.Priority,
		CategoryID:       t.CategoryID,
		CreatedBy:        t.CreatedBy,
		AssignedTo:       t.AssignedTo,
		AssignedAt:       t.AssignedAt,
		ResponseDueAt:    t.ResponseDueAt,
		ResolutionDueAt:  t.ResolutionDueAt,
		FirstRespondedAt: t.FirstRespondedAt,
		ResolvedAt:       t.ResolvedAt,
		ClosedAt:         t.ClosedAt,
		CreatedAt:        t.CreatedAt,
		UpdatedAt:        t.UpdatedAt,
	}
	return m
}

func (m *TicketModel) ToEntity() *entity.Ticket {
	out := &entity.Ticket{
		ID:               m.ID,
		Code:             m.Code,
		Title:            m.Title,
		Description:      m.Description,
		Status:           m.Status,
		Priority:         m.Priority,
		CategoryID:       m.CategoryID,
		CreatedBy:        m.CreatedBy,
		AssignedTo:       m.AssignedTo,
		AssignedAt:       m.AssignedAt,
		ResponseDueAt:    m.ResponseDueAt,
		ResolutionDueAt:  m.ResolutionDueAt,
		FirstRespondedAt: m.FirstRespondedAt,
		ResolvedAt:       m.ResolvedAt,
		ClosedAt:         m.ClosedAt,
		CreatedAt:        m.CreatedAt,
		UpdatedAt:        m.UpdatedAt,
	}
	if m.DeletedAt.Valid {
		t := m.DeletedAt.Time
		out.DeletedAt = &t
	}
	if m.Category != nil {
		out.Category = m.Category.ToEntity()
	}
	if m.Creator != nil {
		out.Creator = m.Creator.ToEntity()
	}
	if m.Assignee != nil {
		out.Assignee = m.Assignee.ToEntity()
	}
	return out
}
