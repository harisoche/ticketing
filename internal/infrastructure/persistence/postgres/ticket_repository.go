package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ticketing-api/internal/domain"
	"ticketing-api/internal/domain/entity"
	"ticketing-api/internal/domain/repository"
	"ticketing-api/internal/infrastructure/persistence/model"
)

type ticketRepository struct {
	db *gorm.DB
}

func NewTicketRepository(db *gorm.DB) repository.TicketRepository {
	return &ticketRepository{db: db}
}

// allowedSortColumns maps repository-level allow-listed sort keys to their
// real SQL column names. ANY value not in this map is silently coerced to
// the default ("created_at") so unvalidated input cannot reach the ORDER BY.
// The use case rejects unknown values with 422 before they reach here;
// this is defence in depth.
var allowedSortColumns = map[string]string{
	"created_at": "created_at",
	"updated_at": "updated_at",
	"priority":   "priority",
	"status":     "status",
	"code":       "code",
}

// Create inserts a new ticket. The database supplies the code via the
// ticket_code_seq sequence (see migration), then we reload the row to pull
// the generated code back into the entity.
func (r *ticketRepository) Create(ctx context.Context, ticket *entity.Ticket) error {
	if ticket.ID == uuid.Nil {
		ticket.ID = uuid.New()
	}
	m := model.TicketModelFromEntity(ticket)
	if err := dbFrom(ctx, r.db).Create(m).Error; err != nil {
		return err
	}
	// Reload to fetch the DB-generated code and timestamps.
	if err := dbFrom(ctx, r.db).
		Preload("Category").
		Preload("Creator").
		Preload("Assignee").
		First(m, "id = ?", m.ID).Error; err != nil {
		return err
	}
	*ticket = *m.ToEntity()
	return nil
}

func (r *ticketRepository) FindByID(ctx context.Context, id uuid.UUID) (*entity.Ticket, error) {
	var m model.TicketModel
	err := dbFrom(ctx, r.db).
		Preload("Category").
		Preload("Creator").
		Preload("Assignee").
		First(&m, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrTicketNotFound
		}
		return nil, err
	}
	return m.ToEntity(), nil
}

// FindByIDForUpdate acquires a row-level lock so concurrent writers wait.
// Must be called inside an active transaction (otherwise SELECT … FOR
// UPDATE is harmless but the surrounding write won't be atomic).
func (r *ticketRepository) FindByIDForUpdate(ctx context.Context, id uuid.UUID) (*entity.Ticket, error) {
	var m model.TicketModel
	err := dbFrom(ctx, r.db).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&m, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrTicketNotFound
		}
		return nil, err
	}
	return m.ToEntity(), nil
}

func (r *ticketRepository) List(ctx context.Context, p repository.TicketListParam) ([]entity.Ticket, int64, error) {
	q := dbFrom(ctx, r.db).Model(&model.TicketModel{})

	q = applyTicketFilters(q, p)

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	column, ok := allowedSortColumns[p.SortBy]
	if !ok {
		column = "created_at"
	}
	order := strings.ToLower(p.SortOrder)
	if order != "asc" && order != "desc" {
		order = "desc"
	}

	page := p.Page
	if page < 1 {
		page = 1
	}
	per := p.PerPage
	if per < 1 {
		per = 10
	}
	if per > 100 {
		per = 100
	}
	offset := (page - 1) * per

	// Append a stable tie-breaker on id so paging is deterministic even when
	// the primary sort column has duplicates.
	primaryOrder := fmt.Sprintf("%s %s", column, strings.ToUpper(order))
	tieOrder := "id ASC"
	if strings.EqualFold(order, "desc") {
		tieOrder = "id DESC"
	}

	var rows []model.TicketModel
	err := q.
		Preload("Category").
		Preload("Creator").
		Preload("Assignee").
		Order(primaryOrder).
		Order(tieOrder).
		Limit(per).
		Offset(offset).
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	out := make([]entity.Ticket, 0, len(rows))
	for i := range rows {
		out = append(out, *rows[i].ToEntity())
	}
	return out, total, nil
}

func applyTicketFilters(q *gorm.DB, p repository.TicketListParam) *gorm.DB {
	if p.Status != "" {
		q = q.Where("status = ?", p.Status)
	}
	if p.Priority != "" {
		q = q.Where("priority = ?", p.Priority)
	}
	if p.CategoryID != nil {
		q = q.Where("category_id = ?", *p.CategoryID)
	}
	if p.CreatedBy != nil {
		q = q.Where("created_by = ?", *p.CreatedBy)
	}
	if p.AssignedTo != nil {
		q = q.Where("assigned_to = ?", *p.AssignedTo)
	}
	// Scope filters are enforced regardless of the optional CreatedBy /
	// AssignedTo above; they encode the role-based visibility window.
	if p.Scope.CreatorID != nil {
		q = q.Where("created_by = ?", *p.Scope.CreatorID)
	}
	if p.Scope.AssigneeID != nil {
		q = q.Where("assigned_to = ?", *p.Scope.AssigneeID)
	}
	if p.Scope.CreatorOrAssigneeID != nil {
		uid := *p.Scope.CreatorOrAssigneeID
		q = q.Where("(created_by = ? OR assigned_to = ?)", uid, uid)
	}
	if s := strings.TrimSpace(p.Search); s != "" {
		needle := "%" + strings.ToLower(s) + "%"
		q = q.Where("(LOWER(code) LIKE ? OR LOWER(title) LIKE ?)", needle, needle)
	}
	// Phase 6 `q` parameter — case-insensitive ILIKE on title + description.
	if s := strings.TrimSpace(p.Query); s != "" {
		needle := "%" + s + "%"
		q = q.Where("(title ILIKE ? OR description ILIKE ?)", needle, needle)
	}
	if p.CreatedFrom != nil {
		q = q.Where("created_at >= ?", *p.CreatedFrom)
	}
	if p.CreatedTo != nil {
		q = q.Where("created_at <= ?", *p.CreatedTo)
	}
	return q
}

// Update writes the editable fields back. The caller has already authorised
// the change in the use-case layer.
func (r *ticketRepository) Update(ctx context.Context, ticket *entity.Ticket) error {
	updates := map[string]any{
		"title":              ticket.Title,
		"description":        ticket.Description,
		"status":             ticket.Status,
		"priority":           ticket.Priority,
		"category_id":        ticket.CategoryID,
		"assigned_to":        ticket.AssignedTo,
		"response_due_at":    ticket.ResponseDueAt,
		"resolution_due_at":  ticket.ResolutionDueAt,
		"first_responded_at": ticket.FirstRespondedAt,
		"resolved_at":        ticket.ResolvedAt,
		"closed_at":          ticket.ClosedAt,
	}
	res := dbFrom(ctx, r.db).
		Model(&model.TicketModel{}).
		Where("id = ?", ticket.ID).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrTicketNotFound
	}
	// Refresh to pull updated_at / preloads.
	reloaded, err := r.FindByID(ctx, ticket.ID)
	if err != nil {
		return err
	}
	*ticket = *reloaded
	return nil
}

// UpdateAssignment writes assigned_to + assigned_at (only those columns).
// The caller is responsible for choosing the correct values and for opening
// the surrounding transaction.
func (r *ticketRepository) UpdateAssignment(ctx context.Context, ticket *entity.Ticket) error {
	updates := map[string]any{
		"assigned_to": ticket.AssignedTo,
		"assigned_at": ticket.AssignedAt,
	}
	res := dbFrom(ctx, r.db).
		Model(&model.TicketModel{}).
		Where("id = ?", ticket.ID).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrTicketNotFound
	}
	return nil
}

// UpdateStatus writes the status + SLA lifecycle timestamps that move with
// the status (first_responded_at, resolved_at, closed_at). The other SLA
// columns (response_due_at, resolution_due_at) are owned by the create /
// classify flows.
func (r *ticketRepository) UpdateStatus(ctx context.Context, ticket *entity.Ticket) error {
	updates := map[string]any{
		"status":             ticket.Status,
		"first_responded_at": ticket.FirstRespondedAt,
		"resolved_at":        ticket.ResolvedAt,
		"closed_at":          ticket.ClosedAt,
	}
	res := dbFrom(ctx, r.db).
		Model(&model.TicketModel{}).
		Where("id = ?", ticket.ID).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrTicketNotFound
	}
	return nil
}

// Summary returns role-scoped aggregate counts for the dashboard.
//
// One COUNT(*) + three GROUP BY queries are issued; nothing is loaded into
// Go memory beyond the aggregate rows themselves. The scope is applied to
// every query so visibility is enforced uniformly.
func (r *ticketRepository) Summary(ctx context.Context, scope repository.TicketListScope, now time.Time, dueSoonMinutes int) (*repository.TicketSummary, error) {
	summary := &repository.TicketSummary{
		ByStatus:   defaultStatusBuckets(),
		ByPriority: defaultPriorityBuckets(),
		ByCategory: []repository.TicketSummaryCategoryCount{},
	}

	base := func() *gorm.DB {
		// Scope-only filter, plus GORM's automatic deleted_at filter from
		// the model.
		q := dbFrom(ctx, r.db).Model(&model.TicketModel{})
		return applyScopeOnly(q, scope)
	}

	if err := base().Count(&summary.Total).Error; err != nil {
		return nil, err
	}

	type kv struct {
		Key   string
		Total int64
	}
	var statusRows []kv
	if err := base().
		Select("status AS key, COUNT(*) AS total").
		Group("status").
		Scan(&statusRows).Error; err != nil {
		return nil, err
	}
	for _, row := range statusRows {
		summary.ByStatus[row.Key] = row.Total
	}

	var prioRows []kv
	if err := base().
		Select("priority AS key, COUNT(*) AS total").
		Group("priority").
		Scan(&prioRows).Error; err != nil {
		return nil, err
	}
	for _, row := range prioRows {
		summary.ByPriority[row.Key] = row.Total
	}

	type catRow struct {
		CategoryID   uuid.UUID
		CategoryName string
		Total        int64
	}
	var catRows []catRow
	if err := base().
		Select("tickets.category_id AS category_id, ticket_categories.name AS category_name, COUNT(*) AS total").
		Joins("JOIN ticket_categories ON ticket_categories.id = tickets.category_id").
		Group("tickets.category_id, ticket_categories.name").
		Order("total DESC, ticket_categories.name ASC").
		Scan(&catRows).Error; err != nil {
		return nil, err
	}
	summary.ByCategory = make([]repository.TicketSummaryCategoryCount, 0, len(catRows))
	for _, row := range catRows {
		summary.ByCategory = append(summary.ByCategory, repository.TicketSummaryCategoryCount{
			CategoryID:   row.CategoryID,
			CategoryName: row.CategoryName,
			Total:        row.Total,
		})
	}

	// Phase 7 SLA aggregates. All three use the same scope so role
	// visibility is preserved.
	if err := base().
		Where("first_responded_at IS NULL AND response_due_at IS NOT NULL AND response_due_at < ?", now).
		Count(&summary.SLAResponseBreached).Error; err != nil {
		return nil, err
	}
	if err := base().
		Where("resolved_at IS NULL AND resolution_due_at IS NOT NULL AND resolution_due_at < ?", now).
		Count(&summary.SLAResolutionBreached).Error; err != nil {
		return nil, err
	}
	soonCutoff := now.Add(time.Duration(dueSoonMinutes) * time.Minute)
	if err := base().
		Where("resolved_at IS NULL AND resolution_due_at IS NOT NULL AND resolution_due_at >= ? AND resolution_due_at <= ?", now, soonCutoff).
		Count(&summary.SLAResolutionDueSoon).Error; err != nil {
		return nil, err
	}

	return summary, nil
}

// applyScopeOnly applies only the scope clauses (the filter helper also
// honours status/priority/etc; for the dashboard we want pure visibility).
func applyScopeOnly(q *gorm.DB, scope repository.TicketListScope) *gorm.DB {
	if scope.CreatorID != nil {
		q = q.Where("created_by = ?", *scope.CreatorID)
	}
	if scope.AssigneeID != nil {
		q = q.Where("assigned_to = ?", *scope.AssigneeID)
	}
	if scope.CreatorOrAssigneeID != nil {
		uid := *scope.CreatorOrAssigneeID
		q = q.Where("(created_by = ? OR assigned_to = ?)", uid, uid)
	}
	return q
}

func defaultStatusBuckets() map[string]int64 {
	return map[string]int64{
		"open":        0,
		"in_progress": 0,
		"resolved":    0,
		"closed":      0,
		"reopened":    0,
	}
}

func defaultPriorityBuckets() map[string]int64 {
	return map[string]int64{
		"low":    0,
		"medium": 0,
		"high":   0,
		"urgent": 0,
	}
}

// SoftDelete sets deleted_at via GORM's soft-delete mechanism.
func (r *ticketRepository) SoftDelete(ctx context.Context, id uuid.UUID) error {
	res := dbFrom(ctx, r.db).Delete(&model.TicketModel{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return domain.ErrTicketNotFound
	}
	return nil
}
