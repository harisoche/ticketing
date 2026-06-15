package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"ticketing-api/internal/domain/entity"
)

// NotificationListFilter is the normalised pagination + unread filter.
type NotificationListFilter struct {
	Page       int
	PerPage    int
	UnreadOnly bool
}

// NotificationRepository handles in-app notification rows. CreateMany is
// expected to be called inside an active transaction (the implementation
// extracts the tx via context).
type NotificationRepository interface {
	CreateMany(ctx context.Context, notifications []entity.Notification) error

	// ListByRecipient returns rows, total matching the filter, and the
	// recipient's overall unread count (regardless of the page).
	ListByRecipient(ctx context.Context, recipientID int64, filter NotificationListFilter) ([]entity.Notification, int64, int64, error)

	// FindByID does not enforce ownership — the use case checks the
	// recipient matches the actor.
	FindByID(ctx context.Context, id uuid.UUID) (*entity.Notification, error)

	// MarkRead is idempotent. Returns ErrNotificationNotFound when the row
	// does not belong to the recipient or doesn't exist.
	MarkRead(ctx context.Context, recipientID int64, notificationID uuid.UUID, readAt time.Time) error

	// MarkAllRead marks every still-unread row belonging to the recipient.
	MarkAllRead(ctx context.Context, recipientID int64, readAt time.Time) (int64, error)

	// UnreadCount returns the recipient's overall unread count.
	UnreadCount(ctx context.Context, recipientID int64) (int64, error)
}
