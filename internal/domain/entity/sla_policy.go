package entity

import (
	"time"

	"github.com/google/uuid"
)

// SLAPolicy is one row of the seeded sla_policies table. Phase 7 stores a
// single active row per priority; admin CRUD is out of scope.
type SLAPolicy struct {
	ID                uuid.UUID
	Priority          string
	ResponseMinutes   int
	ResolutionMinutes int
	IsActive          bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
