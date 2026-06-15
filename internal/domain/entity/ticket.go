package entity

import (
	"time"

	"github.com/google/uuid"
)

// Ticket status values. Persisted as VARCHAR via chk_tickets_status.
const (
	TicketStatusOpen       = "open"
	TicketStatusInProgress = "in_progress"
	TicketStatusResolved   = "resolved"
	TicketStatusClosed     = "closed"
	TicketStatusReopened   = "reopened"
)

// Ticket priority values. Persisted as VARCHAR via chk_tickets_priority.
const (
	TicketPriorityLow    = "low"
	TicketPriorityMedium = "medium"
	TicketPriorityHigh   = "high"
	TicketPriorityUrgent = "urgent"
)

// IsValidTicketStatus reports whether s is a recognised ticket status.
func IsValidTicketStatus(s string) bool {
	switch s {
	case TicketStatusOpen, TicketStatusInProgress, TicketStatusResolved, TicketStatusClosed, TicketStatusReopened:
		return true
	}
	return false
}

// IsValidTicketPriority reports whether p is a recognised ticket priority.
func IsValidTicketPriority(p string) bool {
	switch p {
	case TicketPriorityLow, TicketPriorityMedium, TicketPriorityHigh, TicketPriorityUrgent:
		return true
	}
	return false
}

// Ticket is the framework-independent ticket entity. created_by and
// assigned_to are int64 to match Phase 1's users.id type.
type Ticket struct {
	ID          uuid.UUID
	Code        string
	Title       string
	Description string
	Status      string
	Priority    string
	CategoryID  uuid.UUID
	CreatedBy   int64
	AssignedTo  *int64
	AssignedAt  *time.Time
	Category    *TicketCategory
	Creator     *User
	Assignee    *User
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time

	// Phase 7 SLA + lifecycle timestamps. Nullable; derived state is
	// computed at read time by the use case layer.
	ResponseDueAt    *time.Time
	ResolutionDueAt  *time.Time
	FirstRespondedAt *time.Time
	ResolvedAt       *time.Time
	ClosedAt         *time.Time
}

// CanTransitionTo reports whether moving from t.Status to next is permitted
// by the workflow defined in §5.2 of PHASE_2_TICKET_MANAGEMENT.md.
func (t *Ticket) CanTransitionTo(next string) bool {
	return AllowedTicketTransition(t.Status, next)
}

// AllowedTicketTransition is the Phase 3 workflow:
//
//	open        -> in_progress
//	in_progress -> resolved
//	resolved    -> in_progress, closed
//	closed      -> reopened
//	reopened    -> in_progress
//
// Same-status transitions are not allowed.
func AllowedTicketTransition(current, next string) bool {
	if !IsValidTicketStatus(current) || !IsValidTicketStatus(next) {
		return false
	}
	if current == next {
		return false
	}
	switch current {
	case TicketStatusOpen:
		return next == TicketStatusInProgress
	case TicketStatusInProgress:
		return next == TicketStatusResolved
	case TicketStatusResolved:
		return next == TicketStatusInProgress || next == TicketStatusClosed
	case TicketStatusClosed:
		return next == TicketStatusReopened
	case TicketStatusReopened:
		return next == TicketStatusInProgress
	}
	return false
}
