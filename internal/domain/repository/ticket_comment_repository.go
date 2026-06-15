package repository

import (
	"context"

	"github.com/google/uuid"

	"ticketing-api/internal/domain/entity"
)

// TicketCommentRepository persists ticket comments. List and Find should
// preload the author so the use-case layer doesn't have to issue N+1 queries
// for user summaries.
type TicketCommentRepository interface {
	Create(ctx context.Context, comment *entity.TicketComment) error
	ListByTicketID(ctx context.Context, ticketID uuid.UUID) ([]entity.TicketComment, error)
	FindByID(ctx context.Context, commentID uuid.UUID) (*entity.TicketComment, error)
	Update(ctx context.Context, comment *entity.TicketComment) error
	Delete(ctx context.Context, commentID uuid.UUID) error
}
