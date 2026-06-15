package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"ticketing-api/internal/domain/entity"
)

// TicketListScope encodes the role-aware visibility window. The use case
// decides which scope an actor receives and the repository applies it. A
// nil value (zero-value receiver) means "no scope filter" (admin-style).
type TicketListScope struct {
	// CreatorID, when non-nil, restricts to tickets where created_by matches.
	CreatorID *int64
	// AssigneeID, when non-nil, restricts to tickets where assigned_to matches.
	AssigneeID *int64
	// CreatorOrAssigneeID, when non-nil, restricts to tickets where the user
	// is either the creator OR the current assignee (agent default).
	CreatorOrAssigneeID *int64
}

// TicketListParam is the normalised, allow-listed query used by the
// repository. The use-case layer is responsible for validating, defaulting,
// and clamping all fields before passing them in.
type TicketListParam struct {
	Page    int
	PerPage int

	// Search matches code OR title (Phase 2 legacy).
	Search string

	// Query matches title OR description case-insensitively (Phase 6).
	Query string

	Status   string
	Priority string

	CategoryID *uuid.UUID
	CreatedBy  *int64
	AssignedTo *int64

	// CreatedFrom / CreatedTo are inclusive bounds on created_at.
	CreatedFrom *time.Time
	CreatedTo   *time.Time

	// Visibility scope applied after authorization. Use this rather than
	// CreatedBy / AssignedTo when the value must be enforced regardless of
	// the user's input.
	Scope TicketListScope

	// SortBy and SortOrder must already be allow-listed by the caller. The
	// repository will NOT validate them — they are interpolated directly into
	// the ORDER BY clause.
	SortBy    string
	SortOrder string
}

// TicketSummary holds aggregate dashboard counts for a single scope. All
// maps are pre-populated with every known status / priority key so the
// response always carries zero values.
type TicketSummary struct {
	Total      int64
	ByStatus   map[string]int64
	ByPriority map[string]int64
	ByCategory []TicketSummaryCategoryCount

	// Phase 7 SLA aggregates. Computed at the same scope as Total.
	SLAResponseBreached   int64
	SLAResolutionBreached int64
	SLAResolutionDueSoon  int64
}

// TicketSummaryCategoryCount is one row in the `by_category` array.
type TicketSummaryCategoryCount struct {
	CategoryID   uuid.UUID
	CategoryName string
	Total        int64
}

// TicketRepository persists and retrieves tickets. Methods named
// *ForUpdate, UpdateAssignment, and UpdateStatus are intended to be called
// inside an active TxManager.WithinTx; the implementation honours that by
// pulling the transaction handle from ctx.
type TicketRepository interface {
	Create(ctx context.Context, ticket *entity.Ticket) error
	FindByID(ctx context.Context, id uuid.UUID) (*entity.Ticket, error)
	FindByIDForUpdate(ctx context.Context, id uuid.UUID) (*entity.Ticket, error)
	List(ctx context.Context, param TicketListParam) ([]entity.Ticket, int64, error)
	Update(ctx context.Context, ticket *entity.Ticket) error
	UpdateAssignment(ctx context.Context, ticket *entity.Ticket) error
	UpdateStatus(ctx context.Context, ticket *entity.Ticket) error
	SoftDelete(ctx context.Context, id uuid.UUID) error

	// Summary returns dashboard aggregates honouring the supplied scope.
	// `now` is the clock for SLA-related counts (breached / due-soon); the
	// "due soon" window is `now + DueSoonMinutes`.
	Summary(ctx context.Context, scope TicketListScope, now time.Time, dueSoonMinutes int) (*TicketSummary, error)
}
