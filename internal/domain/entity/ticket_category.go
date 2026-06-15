package entity

import (
	"time"

	"github.com/google/uuid"
)

// TicketCategory groups tickets by topic. The slug is the stable
// machine-readable identifier; the name is what users see.
type TicketCategory struct {
	ID          uuid.UUID
	Name        string
	Slug        string
	Description *string
	IsActive    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}
