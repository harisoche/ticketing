package repository

import (
	"context"

	"github.com/google/uuid"

	"ticketing-api/internal/domain/entity"
)

// TicketCategoryRepository reads and writes ticket categories. Phase 2
// only needed ListActive / FindActiveByID; Phase 5 adds admin CRUD.
type TicketCategoryRepository interface {
	ListActive(ctx context.Context) ([]entity.TicketCategory, error)
	FindActiveByID(ctx context.Context, id uuid.UUID) (*entity.TicketCategory, error)

	// Admin-only operations.
	Create(ctx context.Context, category *entity.TicketCategory) error
	List(ctx context.Context, includeInactive bool) ([]entity.TicketCategory, error)
	FindByID(ctx context.Context, id uuid.UUID) (*entity.TicketCategory, error)
	Update(ctx context.Context, category *entity.TicketCategory) error
}
