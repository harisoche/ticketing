package ticket

import (
	"time"

	"github.com/google/uuid"
)

// ---------- Inputs ----------

type CreateTicketInput struct {
	Title       string
	Description string
	CategoryID  uuid.UUID
	Priority    string
}

type UpdateTicketInput struct {
	Title       *string
	Description *string
	CategoryID  *uuid.UUID
	Priority    *string
}

type UpdateStatusInput struct {
	Status string
	Note   *string
}

// AssignInput: AgentID is the destination agent. nil means "unassign" — the
// HTTP layer typically doesn't expose unassign in Phase 3, but the use case
// supports it because the history action enum includes 'unassigned'.
type AssignInput struct {
	AgentID *int64
	Note    *string
}

type ListInput struct {
	Page    int
	PerPage int

	// Legacy Phase 2 keyword (code OR title). Still honoured.
	Search string

	// Phase 6 keyword (title OR description).
	Query string

	Status   string
	Priority string

	CategoryID *uuid.UUID
	CreatedBy  *int64
	AssignedTo *int64

	CreatedFrom *time.Time
	CreatedTo   *time.Time

	// Scope is the Phase 3 visibility window: "", "created_by_me",
	// "assigned_to_me". Customers ignore everything except their own creator
	// filter.
	Scope string

	// View is the Phase 6 alias of Scope and additionally supports "all"
	// (admin only). When both are set, View wins.
	View string

	SortBy    string
	SortOrder string
}

// ---------- Outputs ----------

type CategoryOutput struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description *string   `json:"description,omitempty"`
}

type UserSummary struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Role string `json:"role,omitempty"`
}

type CategorySummary struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	Slug string    `json:"slug"`
}

type TicketOutput struct {
	ID          uuid.UUID        `json:"id"`
	Code        string           `json:"code"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Status      string           `json:"status"`
	Priority    string           `json:"priority"`
	Category    *CategorySummary `json:"category"`
	Creator     *UserSummary     `json:"creator"`
	Assignee    *UserSummary     `json:"assignee"`
	AssignedAt  *time.Time       `json:"assigned_at,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	SLA         *SLAOutput       `json:"sla,omitempty"`
}

// SLAOutput exposes derived SLA fields. ResponseState / ResolutionState are
// one of "pending" / "met" / "breached". IsResolutionOverdue is `true` when
// the ticket is still unresolved and the current clock is past
// resolution_due_at.
type SLAOutput struct {
	ResponseDueAt       *time.Time `json:"response_due_at"`
	ResolutionDueAt     *time.Time `json:"resolution_due_at"`
	FirstRespondedAt    *time.Time `json:"first_responded_at"`
	ResolvedAt          *time.Time `json:"resolved_at"`
	ResponseState       string     `json:"response_state"`
	ResolutionState     string     `json:"resolution_state"`
	IsResolutionOverdue bool       `json:"is_resolution_overdue"`
}

type ListOutput struct {
	Items      []TicketOutput
	Page       int
	PerPage    int
	Total      int64
	TotalPages int
}

type HistoryOutput struct {
	ID          uuid.UUID    `json:"id"`
	TicketID    uuid.UUID    `json:"ticket_id"`
	Actor       *UserSummary `json:"actor"`
	Action      string       `json:"action"`
	OldStatus   *string      `json:"old_status,omitempty"`
	NewStatus   *string      `json:"new_status,omitempty"`
	OldAssignee *UserSummary `json:"old_assignee,omitempty"`
	NewAssignee *UserSummary `json:"new_assignee,omitempty"`
	Note        *string      `json:"note,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
}

// ---------- Phase 4: comments + timeline ----------

type AddCommentInput struct {
	Body string
}

type UpdateCommentInput struct {
	Body string
}

type CommentOutput struct {
	ID        uuid.UUID    `json:"id"`
	TicketID  uuid.UUID    `json:"ticket_id"`
	Author    *UserSummary `json:"author"`
	Body      string       `json:"body"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// TimelineItemOutput is a tagged union of "history" / "comment". Exactly one
// of History / Comment is non-nil; the other is omitted from JSON.
type TimelineItemOutput struct {
	Type       string         `json:"type"`
	OccurredAt time.Time      `json:"occurred_at"`
	History    *HistoryOutput `json:"history,omitempty"`
	Comment    *CommentOutput `json:"comment,omitempty"`
}

// ---------- Phase 5: admin categories ----------

type CreateCategoryInput struct {
	Name        string
	Slug        string
	Description *string
}

type UpdateCategoryInput struct {
	Name        *string
	Slug        *string
	Description *string
	IsActive    *bool
}

type CategoryAdminOutput struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description *string   `json:"description,omitempty"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ---------- Phase 5: classification ----------

type ClassifyTicketInput struct {
	CategoryID *uuid.UUID
	Priority   *string
}

// ---------- Phase 5: attachments ----------

type UploadAttachmentInput struct {
	OriginalFilename string
	MimeType         string
	SizeBytes        int64
	StorageDriver    string
	StoragePath      string
	StoredFilename   string
}

type DashboardSummaryOutput struct {
	TotalTickets        int64                    `json:"total_tickets"`
	ByStatus            map[string]int64         `json:"by_status"`
	ByPriority          map[string]int64         `json:"by_priority"`
	ByCategory          []DashboardCategoryCount `json:"by_category"`
	SLA                 DashboardSLABlock        `json:"sla"`
	UnreadNotifications int64                    `json:"unread_notifications"`
}

type DashboardSLABlock struct {
	ResponseBreached   int64 `json:"response_breached"`
	ResolutionBreached int64 `json:"resolution_breached"`
	ResolutionDueSoon  int64 `json:"resolution_due_soon"`
}

type DashboardCategoryCount struct {
	CategoryID   uuid.UUID `json:"category_id"`
	CategoryName string    `json:"category_name"`
	Total        int64     `json:"total"`
}

// ---------- Phase 7: notifications ----------

type NotificationListInput struct {
	Page       int
	PerPage    int
	UnreadOnly bool
}

type NotificationOutput struct {
	ID        uuid.UUID  `json:"id"`
	TicketID  *uuid.UUID `json:"ticket_id"`
	Type      string     `json:"type"`
	Title     string     `json:"title"`
	Message   string     `json:"message"`
	ReadAt    *time.Time `json:"read_at"`
	CreatedAt time.Time  `json:"created_at"`
}

type NotificationListOutput struct {
	Items       []NotificationOutput
	Page        int
	PerPage     int
	Total       int64
	TotalPages  int
	UnreadTotal int64
}

type AttachmentOutput struct {
	ID               uuid.UUID    `json:"id"`
	TicketID         uuid.UUID    `json:"ticket_id"`
	UploadedBy       *UserSummary `json:"uploaded_by"`
	OriginalFilename string       `json:"original_filename"`
	MimeType         string       `json:"mime_type"`
	SizeBytes        int64        `json:"size_bytes"`
	CreatedAt        time.Time    `json:"created_at"`
}
