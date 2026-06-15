package model

import (
	"time"

	"github.com/google/uuid"

	"ticketing-api/internal/domain/entity"
)

type NotificationModel struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;column:id"`
	RecipientID int64      `gorm:"column:recipient_id;not null"`
	TicketID    *uuid.UUID `gorm:"type:uuid;column:ticket_id"`
	Type        string     `gorm:"column:type;size:50;not null"`
	Title       string     `gorm:"column:title;size:180;not null"`
	Message     string     `gorm:"column:message;type:text;not null"`
	ReadAt      *time.Time `gorm:"column:read_at"`
	CreatedAt   time.Time  `gorm:"column:created_at;not null;autoCreateTime"`
}

func (NotificationModel) TableName() string { return "notifications" }

func NotificationModelFromEntity(n *entity.Notification) *NotificationModel {
	return &NotificationModel{
		ID:          n.ID,
		RecipientID: n.RecipientID,
		TicketID:    n.TicketID,
		Type:        n.Type,
		Title:       n.Title,
		Message:     n.Message,
		ReadAt:      n.ReadAt,
		CreatedAt:   n.CreatedAt,
	}
}

func (m *NotificationModel) ToEntity() *entity.Notification {
	return &entity.Notification{
		ID:          m.ID,
		RecipientID: m.RecipientID,
		TicketID:    m.TicketID,
		Type:        m.Type,
		Title:       m.Title,
		Message:     m.Message,
		ReadAt:      m.ReadAt,
		CreatedAt:   m.CreatedAt,
	}
}
