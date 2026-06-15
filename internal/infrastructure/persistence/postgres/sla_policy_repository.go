package postgres

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"ticketing-api/internal/domain"
	"ticketing-api/internal/domain/entity"
	"ticketing-api/internal/domain/repository"
	"ticketing-api/internal/infrastructure/persistence/model"
)

type slaPolicyRepository struct {
	db *gorm.DB
}

func NewSLAPolicyRepository(db *gorm.DB) repository.SLAPolicyRepository {
	return &slaPolicyRepository{db: db}
}

func (r *slaPolicyRepository) FindActiveByPriority(ctx context.Context, priority string) (*entity.SLAPolicy, error) {
	var m model.SLAPolicyModel
	err := dbFrom(ctx, r.db).
		Where("priority = ? AND is_active = TRUE", priority).
		First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrSLAPolicyNotFound
		}
		return nil, err
	}
	return m.ToEntity(), nil
}
