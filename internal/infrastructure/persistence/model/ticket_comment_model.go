package model

import (
	"time"

	"github.com/google/uuid"

	"ticketing-api/internal/domain/entity"
)

// TicketCommentModel is the GORM row for ticket_comments.
type TicketCommentModel struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;column:id"`
	TicketID  uuid.UUID `gorm:"type:uuid;column:ticket_id;not null"`
	AuthorID  int64     `gorm:"column:author_id;not null"`
	Body      string    `gorm:"column:body;type:text;not null"`
	CreatedAt time.Time `gorm:"column:created_at;not null;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;autoUpdateTime"`

	Author *UserModel `gorm:"foreignKey:AuthorID;references:ID"`
}

func (TicketCommentModel) TableName() string { return "ticket_comments" }

func TicketCommentModelFromEntity(c *entity.TicketComment) *TicketCommentModel {
	return &TicketCommentModel{
		ID:        c.ID,
		TicketID:  c.TicketID,
		AuthorID:  c.AuthorID,
		Body:      c.Body,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

func (m *TicketCommentModel) ToEntity() *entity.TicketComment {
	out := &entity.TicketComment{
		ID:        m.ID,
		TicketID:  m.TicketID,
		AuthorID:  m.AuthorID,
		Body:      m.Body,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
	if m.Author != nil {
		out.Author = m.Author.ToEntity()
	}
	return out
}
