package entity

import (
	"time"

	"github.com/google/uuid"
)

// Ticket history actions. Persisted via chk_ticket_histories_action.
const (
	TicketHistoryActionCreated       = "created"
	TicketHistoryActionAssigned      = "assigned"
	TicketHistoryActionReassigned    = "reassigned"
	TicketHistoryActionUnassigned    = "unassigned"
	TicketHistoryActionStatusChanged = "status_changed"
)

// IsValidTicketHistoryAction reports whether a is a recognised action.
func IsValidTicketHistoryAction(a string) bool {
	switch a {
	case TicketHistoryActionCreated,
		TicketHistoryActionAssigned,
		TicketHistoryActionReassigned,
		TicketHistoryActionUnassigned,
		TicketHistoryActionStatusChanged:
		return true
	}
	return false
}

// TicketHistory is one audit row recorded inside the same transaction that
// mutated the ticket. Fields that don't apply to a given action stay nil.
type TicketHistory struct {
	ID            uuid.UUID
	TicketID      uuid.UUID
	ActorID       int64
	Action        string
	OldStatus     *string
	NewStatus     *string
	OldAssigneeID *int64
	NewAssigneeID *int64
	Note          *string
	CreatedAt     time.Time

	Actor       *User
	OldAssignee *User
	NewAssignee *User
}
