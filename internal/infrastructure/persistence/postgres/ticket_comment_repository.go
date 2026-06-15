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

type ticketCommentRepository struct {
	db *gorm.DB
}

func NewTicketCommentRepository(db *gorm.DB) repository.TicketCommentRepository {
	return &ticketCommentRepository{db: db}
}

func (r *ticketCommentRepository) Create(ctx context.Context, c *entity.TicketComment) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	m := model.TicketCommentModelFromEntity(c)
	if err := dbFrom(ctx, r.db).Create(m).Error; err != nil {
		return err
	}
	// Reload with author preload so the caller has display data ready.
	if err := dbFrom(ctx, r.db).
		Preload("Author").
		First(m, "id = ?", m.ID).Error; err != nil {
		return err
	}
	*c = *m.ToEntity()
	return nil
}

func (r *ticketCommentRepository) ListByTicketID(ctx context.Context, ticketID uuid.UUID) ([]entity.TicketComment, error) {
	var rows []model.TicketCommentModel
	err := dbFrom(ctx, r.db).
		Preload("Author").
		Where("ticket_id = ?", ticketID).
		Order("created_at ASC, id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]entity.TicketComment, 0, len(rows))
	for i := range rows {
		out = append(out, *rows[i].ToEntity())
	}
	return out, nil
}

func (r *ticketCommentRepository) FindByID(ctx context.Context, id uuid.UUID) (*entity.TicketComment, error) {
	var m model.TicketCommentModel
	err := dbFrom(ctx, r.db).
		Preload("Author").
		First(&m, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrCommentNotFound
		}
		return nil, err
	}
	return m.ToEntity(), nil
}

// Update writes the body (and refreshes updated_at via autoUpdateTime).
func (r *ticketCommentRepository) Update(ctx context.Context, c *entity.TicketComment) error {
	res := dbFrom(ctx, r.db).
		Model(&model.TicketCommentModel{}).
		Where("id = ?", c.ID).
		Update("body", c.Body)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrCommentNotFound
	}
	// Refresh so the caller sees the bumped updated_at + preload.
	reloaded, err := r.FindByID(ctx, c.ID)
	if err != nil {
		return err
	}
	*c = *reloaded
	return nil
}

// Delete is hard delete per Phase 4 spec (no deleted_at column).
func (r *ticketCommentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	res := dbFrom(ctx, r.db).Delete(&model.TicketCommentModel{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrCommentNotFound
	}
	return nil
}
