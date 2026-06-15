package postgres

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ticketing-api/internal/domain/entity"
	"ticketing-api/internal/domain/repository"
	"ticketing-api/internal/infrastructure/persistence/model"
)

type ticketHistoryRepository struct {
	db *gorm.DB
}

func NewTicketHistoryRepository(db *gorm.DB) repository.TicketHistoryRepository {
	return &ticketHistoryRepository{db: db}
}

func (r *ticketHistoryRepository) Create(ctx context.Context, h *entity.TicketHistory) error {
	if h.ID == uuid.Nil {
		h.ID = uuid.New()
	}
	m := model.TicketHistoryModelFromEntity(h)
	if err := dbFrom(ctx, r.db).Create(m).Error; err != nil {
		return err
	}
	h.CreatedAt = m.CreatedAt
	return nil
}

func (r *ticketHistoryRepository) ListByTicketID(ctx context.Context, ticketID uuid.UUID) ([]entity.TicketHistory, error) {
	var rows []model.TicketHistoryModel
	err := dbFrom(ctx, r.db).
		Preload("Actor").
		Preload("OldAssignee").
		Preload("NewAssignee").
		Where("ticket_id = ?", ticketID).
		Order("created_at ASC, id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]entity.TicketHistory, 0, len(rows))
	for i := range rows {
		out = append(out, *rows[i].ToEntity())
	}
	return out, nil
}
