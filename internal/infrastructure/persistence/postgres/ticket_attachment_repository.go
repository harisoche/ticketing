package postgres

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"ticketing-api/internal/domain"
	"ticketing-api/internal/domain/entity"
	"ticketing-api/internal/domain/repository"
	"ticketing-api/internal/infrastructure/persistence/model"
)

type ticketAttachmentRepository struct {
	db *gorm.DB
}

func NewTicketAttachmentRepository(db *gorm.DB) repository.TicketAttachmentRepository {
	return &ticketAttachmentRepository{db: db}
}

func (r *ticketAttachmentRepository) Create(ctx context.Context, a *entity.TicketAttachment) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	m := model.TicketAttachmentModelFromEntity(a)
	if err := dbFrom(ctx, r.db).Create(m).Error; err != nil {
		return err
	}
	a.CreatedAt = m.CreatedAt
	return nil
}

func (r *ticketAttachmentRepository) ListByTicketID(ctx context.Context, ticketID uuid.UUID) ([]entity.TicketAttachment, error) {
	var rows []model.TicketAttachmentModel
	err := dbFrom(ctx, r.db).
		Preload("Uploader").
		Where("ticket_id = ?", ticketID).
		Order("created_at ASC, id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]entity.TicketAttachment, 0, len(rows))
	for i := range rows {
		out = append(out, *rows[i].ToEntity())
	}
	return out, nil
}

func (r *ticketAttachmentRepository) FindByID(ctx context.Context, id uuid.UUID) (*entity.TicketAttachment, error) {
	var m model.TicketAttachmentModel
	err := dbFrom(ctx, r.db).
		Preload("Uploader").
		First(&m, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrAttachmentNotFound
		}
		return nil, err
	}
	return m.ToEntity(), nil
}

func (r *ticketAttachmentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res := dbFrom(ctx, r.db).Delete(&model.TicketAttachmentModel{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrAttachmentNotFound
	}
	return nil
}
