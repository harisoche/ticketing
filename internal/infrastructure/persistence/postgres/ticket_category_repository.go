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

type ticketCategoryRepository struct {
	db *gorm.DB
}

func NewTicketCategoryRepository(db *gorm.DB) repository.TicketCategoryRepository {
	return &ticketCategoryRepository{db: db}
}

func (r *ticketCategoryRepository) ListActive(ctx context.Context) ([]entity.TicketCategory, error) {
	var rows []model.TicketCategoryModel
	if err := r.db.WithContext(ctx).
		Where("is_active = TRUE").
		Order("LOWER(name) ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]entity.TicketCategory, 0, len(rows))
	for i := range rows {
		out = append(out, *rows[i].ToEntity())
	}
	return out, nil
}

func (r *ticketCategoryRepository) FindActiveByID(ctx context.Context, id uuid.UUID) (*entity.TicketCategory, error) {
	var row model.TicketCategoryModel
	err := r.db.WithContext(ctx).
		Where("id = ? AND is_active = TRUE", id).
		First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrTicketCategoryNotFound
		}
		return nil, err
	}
	return row.ToEntity(), nil
}

func (r *ticketCategoryRepository) Create(ctx context.Context, c *entity.TicketCategory) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	m := &model.TicketCategoryModel{
		ID:          c.ID,
		Name:        c.Name,
		Slug:        c.Slug,
		Description: c.Description,
		IsActive:    c.IsActive,
	}
	if err := r.db.WithContext(ctx).Create(m).Error; err != nil {
		if isUniqueViolation(err) {
			return domain.ErrCategoryConflict
		}
		return err
	}
	c.CreatedAt = m.CreatedAt
	c.UpdatedAt = m.UpdatedAt
	return nil
}

func (r *ticketCategoryRepository) List(ctx context.Context, includeInactive bool) ([]entity.TicketCategory, error) {
	q := r.db.WithContext(ctx).Order("LOWER(name) ASC")
	if !includeInactive {
		q = q.Where("is_active = TRUE")
	}
	var rows []model.TicketCategoryModel
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]entity.TicketCategory, 0, len(rows))
	for i := range rows {
		out = append(out, *rows[i].ToEntity())
	}
	return out, nil
}

func (r *ticketCategoryRepository) FindByID(ctx context.Context, id uuid.UUID) (*entity.TicketCategory, error) {
	var row model.TicketCategoryModel
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrTicketCategoryNotFound
		}
		return nil, err
	}
	return row.ToEntity(), nil
}

func (r *ticketCategoryRepository) Update(ctx context.Context, c *entity.TicketCategory) error {
	updates := map[string]any{
		"name":        c.Name,
		"slug":        c.Slug,
		"description": c.Description,
		"is_active":   c.IsActive,
	}
	res := r.db.WithContext(ctx).
		Model(&model.TicketCategoryModel{}).
		Where("id = ?", c.ID).
		Updates(updates)
	if res.Error != nil {
		if isUniqueViolation(res.Error) {
			return domain.ErrCategoryConflict
		}
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrTicketCategoryNotFound
	}
	// Reload to pick up updated_at
	var reloaded model.TicketCategoryModel
	if err := r.db.WithContext(ctx).First(&reloaded, "id = ?", c.ID).Error; err != nil {
		return err
	}
	*c = *reloaded.ToEntity()
	return nil
}
