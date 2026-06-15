package entity

import (
	"time"

	"github.com/google/uuid"
)

// Notification action types. Persisted via chk_notifications_type.
const (
	NotificationTypeTicketAssigned      = "ticket_assigned"
	NotificationTypeTicketReassigned    = "ticket_reassigned"
	NotificationTypeTicketStatusChanged = "ticket_status_changed"
	NotificationTypeTicketCommented     = "ticket_commented"
)

// IsValidNotificationType reports whether t is a recognised notification kind.
func IsValidNotificationType(t string) bool {
	switch t {
	case NotificationTypeTicketAssigned,
		NotificationTypeTicketReassigned,
		NotificationTypeTicketStatusChanged,
		NotificationTypeTicketCommented:
		return true
	}
	return false
}

// Notification is one in-app message addressed to a user.
type Notification struct {
	ID          uuid.UUID
	RecipientID int64
	TicketID    *uuid.UUID
	Type        string
	Title       string
	Message     string
	ReadAt      *time.Time
	CreatedAt   time.Time
}
