package repository

import (
	"context"

	"github.com/google/uuid"

	"ticketing-api/internal/domain/entity"
)

// TicketHistoryRepository writes and reads workflow-history rows.
//
// Create is called inside an active transaction (the implementation looks
// up the tx via context).
type TicketHistoryRepository interface {
	Create(ctx context.Context, history *entity.TicketHistory) error
	ListByTicketID(ctx context.Context, ticketID uuid.UUID) ([]entity.TicketHistory, error)
}
