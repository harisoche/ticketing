package model

import (
	"time"

	"github.com/google/uuid"

	"ticketing-api/internal/domain/entity"
)

// TicketHistoryModel is the GORM-bound row for ticket_histories.
type TicketHistoryModel struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey;column:id"`
	TicketID      uuid.UUID `gorm:"type:uuid;column:ticket_id;not null"`
	ActorID       int64     `gorm:"column:actor_id;not null"`
	Action        string    `gorm:"column:action;size:30;not null"`
	OldStatus     *string   `gorm:"column:old_status;size:30"`
	NewStatus     *string   `gorm:"column:new_status;size:30"`
	OldAssigneeID *int64    `gorm:"column:old_assignee_id"`
	NewAssigneeID *int64    `gorm:"column:new_assignee_id"`
	Note          *string   `gorm:"column:note"`
	CreatedAt     time.Time `gorm:"column:created_at;not null;autoCreateTime"`

	Actor       *UserModel `gorm:"foreignKey:ActorID;references:ID"`
	OldAssignee *UserModel `gorm:"foreignKey:OldAssigneeID;references:ID"`
	NewAssignee *UserModel `gorm:"foreignKey:NewAssigneeID;references:ID"`
}

func (TicketHistoryModel) TableName() string { return "ticket_histories" }

func TicketHistoryModelFromEntity(h *entity.TicketHistory) *TicketHistoryModel {
	return &TicketHistoryModel{
		ID:            h.ID,
		TicketID:      h.TicketID,
		ActorID:       h.ActorID,
		Action:        h.Action,
		OldStatus:     h.OldStatus,
		NewStatus:     h.NewStatus,
		OldAssigneeID: h.OldAssigneeID,
		NewAssigneeID: h.NewAssigneeID,
		Note:          h.Note,
		CreatedAt:     h.CreatedAt,
	}
}

func (m *TicketHistoryModel) ToEntity() *entity.TicketHistory {
	out := &entity.TicketHistory{
		ID:            m.ID,
		TicketID:      m.TicketID,
		ActorID:       m.ActorID,
		Action:        m.Action,
		OldStatus:     m.OldStatus,
		NewStatus:     m.NewStatus,
		OldAssigneeID: m.OldAssigneeID,
		NewAssigneeID: m.NewAssigneeID,
		Note:          m.Note,
		CreatedAt:     m.CreatedAt,
	}
	if m.Actor != nil {
		out.Actor = m.Actor.ToEntity()
	}
	if m.OldAssignee != nil {
		out.OldAssignee = m.OldAssignee.ToEntity()
	}
	if m.NewAssignee != nil {
		out.NewAssignee = m.NewAssignee.ToEntity()
	}
	return out
}
