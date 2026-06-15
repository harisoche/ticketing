package repository

import (
	"context"

	"ticketing-api/internal/domain/entity"
)

// SLAPolicyRepository exposes the seeded SLA policy table. Phase 7 only
// needs the read path; admin write is out of scope.
type SLAPolicyRepository interface {
	FindActiveByPriority(ctx context.Context, priority string) (*entity.SLAPolicy, error)
}
