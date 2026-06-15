package ticket

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"ticketing-api/internal/domain"
	"ticketing-api/internal/domain/entity"
	"ticketing-api/internal/domain/repository"
)

// ---------- Phase 7 wiring ----------

// WithPhase7 attaches the SLA and notification repositories. Like Phase 5,
// this preserves the older constructor signature so existing tests don't
// need to widen their fakes.
func (s *Service) WithPhase7(
	policies repository.SLAPolicyRepository,
	notifications repository.NotificationRepository,
) *Service {
	cp := *s
	cp.policies = policies
	cp.notifications = notifications
	return &cp
}

// ---------- SLA computation ----------

// SLA state values, mapped exactly to the spec.
const (
	SLAStatePending  = "pending"
	SLAStateMet      = "met"
	SLAStateBreached = "breached"
)

// computeSLAOutput derives the read-time SLA view for a ticket.
func computeSLAOutput(t *entity.Ticket, now time.Time) *SLAOutput {
	if t.ResponseDueAt == nil && t.ResolutionDueAt == nil {
		return nil
	}
	out := &SLAOutput{
		ResponseDueAt:    t.ResponseDueAt,
		ResolutionDueAt:  t.ResolutionDueAt,
		FirstRespondedAt: t.FirstRespondedAt,
		ResolvedAt:       t.ResolvedAt,
	}
	out.ResponseState = computeResponseState(t.FirstRespondedAt, t.ResponseDueAt, now)
	out.ResolutionState = computeResolutionState(t.ResolvedAt, t.ResolutionDueAt, now)
	if t.ResolvedAt == nil && t.ResolutionDueAt != nil && now.After(*t.ResolutionDueAt) {
		out.IsResolutionOverdue = true
	}
	return out
}

func computeResponseState(firstResponse, due *time.Time, now time.Time) string {
	if due == nil {
		return SLAStatePending
	}
	if firstResponse != nil {
		if firstResponse.After(*due) {
			return SLAStateBreached
		}
		return SLAStateMet
	}
	if now.After(*due) {
		return SLAStateBreached
	}
	return SLAStatePending
}

func computeResolutionState(resolved, due *time.Time, now time.Time) string {
	if due == nil {
		return SLAStatePending
	}
	if resolved != nil {
		if resolved.After(*due) {
			return SLAStateBreached
		}
		return SLAStateMet
	}
	if now.After(*due) {
		return SLAStateBreached
	}
	return SLAStatePending
}

// applyPolicyToTicket sets response_due_at / resolution_due_at on the ticket
// based on the policy and the ticket's created_at. Used by Create and by
// ClassifyTicket when priority changes.
func applyPolicyToTicket(t *entity.Ticket, p *entity.SLAPolicy) {
	if p == nil {
		return
	}
	response := t.CreatedAt.Add(time.Duration(p.ResponseMinutes) * time.Minute)
	resolution := t.CreatedAt.Add(time.Duration(p.ResolutionMinutes) * time.Minute)
	t.ResponseDueAt = &response
	t.ResolutionDueAt = &resolution
}

// ---------- Notification helpers ----------

// notify inserts notifications inside the current transaction. The caller is
// responsible for skipping recipients that equal the actor (self-suppression).
func (s *Service) notify(ctx context.Context, notifications []entity.Notification) error {
	if s.notifications == nil || len(notifications) == 0 {
		return nil
	}
	return s.notifications.CreateMany(ctx, notifications)
}

func notificationForAssignment(actor *entity.User, t *entity.Ticket, recipientID int64, action string) entity.Notification {
	ticketID := t.ID
	var title, msg string
	switch action {
	case entity.NotificationTypeTicketAssigned:
		title = "New ticket assignment"
		msg = fmt.Sprintf("Ticket %s has been assigned to you: %s", t.Code, t.Title)
	case entity.NotificationTypeTicketReassigned:
		title = "Ticket reassigned to you"
		msg = fmt.Sprintf("Ticket %s has been reassigned to you: %s", t.Code, t.Title)
	}
	_ = actor
	return entity.Notification{
		RecipientID: recipientID,
		TicketID:    &ticketID,
		Type:        action,
		Title:       title,
		Message:     msg,
	}
}

func notificationForStatusChange(actor *entity.User, t *entity.Ticket, recipientID int64, oldStatus, newStatus string) entity.Notification {
	ticketID := t.ID
	return entity.Notification{
		RecipientID: recipientID,
		TicketID:    &ticketID,
		Type:        entity.NotificationTypeTicketStatusChanged,
		Title:       "Ticket status updated",
		Message: fmt.Sprintf("Ticket %s status changed from %s to %s by %s",
			t.Code, oldStatus, newStatus, actor.Name),
	}
}

func notificationForComment(actor *entity.User, t *entity.Ticket, recipientID int64) entity.Notification {
	ticketID := t.ID
	return entity.Notification{
		RecipientID: recipientID,
		TicketID:    &ticketID,
		Type:        entity.NotificationTypeTicketCommented,
		Title:       "New comment on ticket",
		Message:     fmt.Sprintf("%s commented on ticket %s", actor.Name, t.Code),
	}
}

// ---------- Notification use cases ----------

func (s *Service) ListNotifications(ctx context.Context, actor *entity.User, in NotificationListInput) (*NotificationListOutput, error) {
	if s.notifications == nil {
		return nil, errors.New("notifications repository not wired")
	}
	page := in.Page
	if page == 0 {
		page = 1
	}
	if page < 1 {
		return nil, domain.ErrInvalidInput
	}
	per := in.PerPage
	if per == 0 {
		per = defaultPerPage
	}
	if per < 1 {
		return nil, domain.ErrInvalidInput
	}
	if per > maxPerPage {
		per = maxPerPage
	}
	rows, total, unread, err := s.notifications.ListByRecipient(ctx, actor.ID, repository.NotificationListFilter{
		Page: page, PerPage: per, UnreadOnly: in.UnreadOnly,
	})
	if err != nil {
		return nil, err
	}
	items := make([]NotificationOutput, 0, len(rows))
	for i := range rows {
		items = append(items, toNotificationOutput(&rows[i]))
	}
	totalPages := 0
	if per > 0 {
		totalPages = int((total + int64(per) - 1) / int64(per))
	}
	return &NotificationListOutput{
		Items: items, Page: page, PerPage: per, Total: total,
		TotalPages: totalPages, UnreadTotal: unread,
	}, nil
}

func (s *Service) MarkNotificationRead(ctx context.Context, actor *entity.User, id uuid.UUID) error {
	if s.notifications == nil {
		return errors.New("notifications repository not wired")
	}
	return s.notifications.MarkRead(ctx, actor.ID, id, s.now())
}

func (s *Service) MarkAllNotificationsRead(ctx context.Context, actor *entity.User) (int64, error) {
	if s.notifications == nil {
		return 0, errors.New("notifications repository not wired")
	}
	return s.notifications.MarkAllRead(ctx, actor.ID, s.now())
}

func toNotificationOutput(n *entity.Notification) NotificationOutput {
	return NotificationOutput{
		ID:        n.ID,
		TicketID:  n.TicketID,
		Type:      n.Type,
		Title:     n.Title,
		Message:   n.Message,
		ReadAt:    n.ReadAt,
		CreatedAt: n.CreatedAt,
	}
}
