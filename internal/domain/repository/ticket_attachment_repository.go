package repository

import (
	"context"

	"github.com/google/uuid"

	"ticketing-api/internal/domain/entity"
)

// TicketAttachmentRepository persists attachment metadata. The actual file
// bytes live behind the FileStorage interface.
type TicketAttachmentRepository interface {
	Create(ctx context.Context, attachment *entity.TicketAttachment) error
	ListByTicketID(ctx context.Context, ticketID uuid.UUID) ([]entity.TicketAttachment, error)
	FindByID(ctx context.Context, attachmentID uuid.UUID) (*entity.TicketAttachment, error)
	Delete(ctx context.Context, attachmentID uuid.UUID) error
}
