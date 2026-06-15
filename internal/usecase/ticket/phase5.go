package ticket

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"ticketing-api/internal/domain"
	"ticketing-api/internal/domain/entity"
	"ticketing-api/internal/domain/repository"
	"ticketing-api/internal/domain/service"
)

// ---------- Phase 5 dependencies wired onto Service via WithPhase5() ----------

// WithPhase5 returns a copy of the Service that knows about attachments,
// file storage, and admin category mutations. Doing this through a setter
// (rather than another argument to NewService) keeps Phase 1–4 constructors
// untouched and lets tests build a Phase 5 harness without rewriting the
// older fakes.
func (s *Service) WithPhase5(
	attachments repository.TicketAttachmentRepository,
	storage service.FileStorage,
	maxUploadBytes int64,
) *Service {
	cp := *s
	cp.attachments = attachments
	cp.storage = storage
	cp.maxUploadBytes = maxUploadBytes
	return &cp
}

const minNameLength = 2
const minSlugLength = 2
const maxNameLengthCategory = 100
const maxSlugLength = 120

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// ---------- Admin category CRUD ----------

func (s *Service) ListPublicCategories(ctx context.Context) ([]CategoryOutput, error) {
	return s.ListActiveCategories(ctx)
}

func (s *Service) AdminListCategories(ctx context.Context, actor *entity.User) ([]CategoryAdminOutput, error) {
	if actor.Role != entity.RoleAdmin {
		return nil, domain.ErrForbidden
	}
	rows, err := s.categories.List(ctx, true)
	if err != nil {
		return nil, err
	}
	out := make([]CategoryAdminOutput, 0, len(rows))
	for i := range rows {
		out = append(out, toCategoryAdminOutput(&rows[i]))
	}
	return out, nil
}

func (s *Service) AdminCreateCategory(ctx context.Context, actor *entity.User, in CreateCategoryInput) (*CategoryAdminOutput, error) {
	if actor.Role != entity.RoleAdmin {
		return nil, domain.ErrForbidden
	}
	name, slug, desc, err := validateCategoryFields(in.Name, in.Slug, in.Description)
	if err != nil {
		return nil, err
	}
	c := &entity.TicketCategory{
		Name:        name,
		Slug:        slug,
		Description: desc,
		IsActive:    true,
	}
	if err := s.categories.Create(ctx, c); err != nil {
		return nil, err
	}
	out := toCategoryAdminOutput(c)
	return &out, nil
}

func (s *Service) AdminUpdateCategory(ctx context.Context, actor *entity.User, id uuid.UUID, in UpdateCategoryInput) (*CategoryAdminOutput, error) {
	if actor.Role != entity.RoleAdmin {
		return nil, domain.ErrForbidden
	}
	c, err := s.categories.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if len(name) < minNameLength || len(name) > maxNameLengthCategory {
			return nil, domain.ErrInvalidInput
		}
		c.Name = name
	}
	if in.Slug != nil {
		slug := strings.TrimSpace(strings.ToLower(*in.Slug))
		if !validSlug(slug) {
			return nil, domain.ErrInvalidInput
		}
		c.Slug = slug
	}
	if in.Description != nil {
		desc := strings.TrimSpace(*in.Description)
		if desc == "" {
			c.Description = nil
		} else {
			c.Description = &desc
		}
	}
	if in.IsActive != nil {
		c.IsActive = *in.IsActive
	}
	if err := s.categories.Update(ctx, c); err != nil {
		return nil, err
	}
	out := toCategoryAdminOutput(c)
	return &out, nil
}

// AdminDeactivateCategory flips is_active to false. Categories are never
// hard-deleted because existing tickets continue to point at them.
func (s *Service) AdminDeactivateCategory(ctx context.Context, actor *entity.User, id uuid.UUID) (*CategoryAdminOutput, error) {
	if actor.Role != entity.RoleAdmin {
		return nil, domain.ErrForbidden
	}
	c, err := s.categories.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	c.IsActive = false
	if err := s.categories.Update(ctx, c); err != nil {
		return nil, err
	}
	out := toCategoryAdminOutput(c)
	return &out, nil
}

func validateCategoryFields(name, slug string, description *string) (string, string, *string, error) {
	n := strings.TrimSpace(name)
	if len(n) < minNameLength || len(n) > maxNameLengthCategory {
		return "", "", nil, domain.ErrInvalidInput
	}
	s := strings.TrimSpace(strings.ToLower(slug))
	if !validSlug(s) {
		return "", "", nil, domain.ErrInvalidInput
	}
	var desc *string
	if description != nil {
		d := strings.TrimSpace(*description)
		if d != "" {
			desc = &d
		}
	}
	return n, s, desc, nil
}

func validSlug(s string) bool {
	if len(s) < minSlugLength || len(s) > maxSlugLength {
		return false
	}
	return slugPattern.MatchString(s)
}

func toCategoryAdminOutput(c *entity.TicketCategory) CategoryAdminOutput {
	return CategoryAdminOutput{
		ID:          c.ID,
		Name:        c.Name,
		Slug:        c.Slug,
		Description: c.Description,
		IsActive:    c.IsActive,
		CreatedAt:   c.CreatedAt,
		UpdatedAt:   c.UpdatedAt,
	}
}

// ---------- Classification ----------

func (s *Service) ClassifyTicket(ctx context.Context, actor *entity.User, id uuid.UUID, in ClassifyTicketInput) (*TicketOutput, error) {
	if in.CategoryID == nil && in.Priority == nil {
		return nil, domain.ErrInvalidInput
	}
	t, err := s.tickets.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canViewTicket(actor, t) {
		return nil, domain.ErrTicketNotFound
	}
	switch actor.Role {
	case entity.RoleAdmin:
		// always allowed
	case entity.RoleAgent:
		if t.AssignedTo == nil || *t.AssignedTo != actor.ID {
			return nil, domain.ErrForbidden
		}
	default:
		return nil, domain.ErrForbidden
	}

	priorityChanged := false
	if in.Priority != nil {
		p := strings.TrimSpace(*in.Priority)
		if !entity.IsValidTicketPriority(p) {
			return nil, domain.ErrInvalidInput
		}
		if p != t.Priority {
			priorityChanged = true
			t.Priority = p
		}
	}
	if in.CategoryID != nil {
		cat, err := s.categories.FindActiveByID(ctx, *in.CategoryID)
		if err != nil {
			return nil, err
		}
		t.CategoryID = cat.ID
	}

	// Phase 7: a priority change recomputes the due times from the ticket's
	// ORIGINAL created_at. Already-recorded first_responded_at / resolved_at
	// / closed_at are intentionally preserved.
	if priorityChanged && s.policies != nil {
		p, perr := s.policies.FindActiveByPriority(ctx, t.Priority)
		if perr == nil {
			applyPolicyToTicket(t, p)
		} else if !errors.Is(perr, domain.ErrSLAPolicyNotFound) {
			return nil, perr
		}
	}

	if err := s.tickets.Update(ctx, t); err != nil {
		return nil, err
	}
	return toOutputAt(t, s.now()), nil
}

// ---------- Attachments ----------

// allowedAttachmentMimeTypes maps detected MIME → file extension used when
// constructing the stored filename.
var allowedAttachmentMimeTypes = map[string]string{
	"application/pdf": "pdf",
	"image/jpeg":      "jpg",
	"image/png":       "png",
	"text/plain":      "txt",
}

// safeFilenamePattern keeps display filenames to safe characters.
var safeFilenamePattern = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// AddAttachment reads body (up to maxUploadBytes), sniffs the MIME type,
// rejects unsupported / oversized payloads, then saves to storage and
// inserts the metadata row. If the DB insert fails the saved file is
// removed so we never leak orphan blobs.
func (s *Service) AddAttachment(ctx context.Context, actor *entity.User, ticketID uuid.UUID, body io.Reader, originalFilename string, declaredSize int64) (*AttachmentOutput, error) {
	if s.attachments == nil || s.storage == nil {
		return nil, errors.New("ticket service: attachments not wired (call WithPhase5)")
	}
	t, err := s.tickets.FindByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if !canViewTicket(actor, t) {
		return nil, domain.ErrTicketNotFound
	}

	// Read body into memory with a hard cap. Use the configured limit + 1 so
	// we can tell a "right-at-the-limit" upload from "exceeds limit".
	limit := s.maxUploadBytes
	if limit <= 0 {
		limit = 5 * 1024 * 1024
	}
	limited := io.LimitReader(body, limit+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(buf)) > limit {
		return nil, domain.ErrAttachmentTooLarge
	}
	if len(buf) == 0 {
		return nil, domain.ErrInvalidInput
	}

	// MIME detection from contents, not from client header. Returns a
	// canonical type that may include `; charset=…`; strip it.
	detected := http.DetectContentType(buf)
	mime := strings.TrimSpace(strings.SplitN(detected, ";", 2)[0])
	ext, ok := allowedAttachmentMimeTypes[mime]
	if !ok {
		return nil, domain.ErrAttachmentUnsupported
	}

	// Display filename: take the last path component, allow only safe chars,
	// truncate at 255 chars, never empty.
	display := sanitiseOriginalFilename(originalFilename)
	if display == "" {
		display = "attachment." + ext
	}

	stored, err := s.storage.Save(ctx, service.SaveFileInput{
		Body:      strings.NewReader(string(buf)),
		SizeHint:  int64(len(buf)),
		MimeType:  mime,
		Extension: ext,
	})
	if err != nil {
		return nil, err
	}

	a := &entity.TicketAttachment{
		TicketID:         ticketID,
		UploadedBy:       actor.ID,
		StorageDriver:    storageDriverName(s.storage),
		StoragePath:      stored.StoragePath,
		OriginalFilename: display,
		StoredFilename:   stored.StoredFilename,
		MimeType:         mime,
		SizeBytes:        int64(len(buf)),
	}
	if err := s.attachments.Create(ctx, a); err != nil {
		// Compensate — remove the stored file so we don't leak storage.
		_ = s.storage.Delete(ctx, stored.StoragePath)
		return nil, err
	}
	if declaredSize > 0 && declaredSize != a.SizeBytes {
		// Not a hard fail — sizing hints are informational.
	}
	a.Uploader = actor
	out := toAttachmentOutput(a)
	return &out, nil
}

func (s *Service) ListAttachments(ctx context.Context, actor *entity.User, ticketID uuid.UUID) ([]AttachmentOutput, error) {
	t, err := s.tickets.FindByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if !canViewTicket(actor, t) {
		return nil, domain.ErrTicketNotFound
	}
	if s.attachments == nil {
		return nil, errors.New("ticket service: attachments not wired (call WithPhase5)")
	}
	rows, err := s.attachments.ListByTicketID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	out := make([]AttachmentOutput, 0, len(rows))
	for i := range rows {
		out = append(out, toAttachmentOutput(&rows[i]))
	}
	return out, nil
}

// DownloadAttachment returns the metadata + an open reader. The handler is
// responsible for closing the reader after streaming.
func (s *Service) DownloadAttachment(ctx context.Context, actor *entity.User, ticketID, attachmentID uuid.UUID) (*AttachmentOutput, io.ReadCloser, error) {
	if s.attachments == nil || s.storage == nil {
		return nil, nil, errors.New("ticket service: attachments not wired (call WithPhase5)")
	}
	t, err := s.tickets.FindByID(ctx, ticketID)
	if err != nil {
		return nil, nil, err
	}
	if !canViewTicket(actor, t) {
		return nil, nil, domain.ErrTicketNotFound
	}
	a, err := s.attachments.FindByID(ctx, attachmentID)
	if err != nil {
		return nil, nil, err
	}
	if a.TicketID != ticketID {
		return nil, nil, domain.ErrAttachmentNotFound
	}
	rc, err := s.storage.Open(ctx, a.StoragePath)
	if err != nil {
		return nil, nil, err
	}
	out := toAttachmentOutput(a)
	return &out, rc, nil
}

// DeleteAttachment removes the DB row then the stored file. The DB is the
// source of truth — if the file delete fails after the row is gone we
// surface the error (the caller can log + alert).
func (s *Service) DeleteAttachment(ctx context.Context, actor *entity.User, ticketID, attachmentID uuid.UUID) error {
	if s.attachments == nil || s.storage == nil {
		return errors.New("ticket service: attachments not wired (call WithPhase5)")
	}
	t, err := s.tickets.FindByID(ctx, ticketID)
	if err != nil {
		return err
	}
	if !canViewTicket(actor, t) {
		return domain.ErrTicketNotFound
	}
	a, err := s.attachments.FindByID(ctx, attachmentID)
	if err != nil {
		return err
	}
	if a.TicketID != ticketID {
		return domain.ErrAttachmentNotFound
	}
	if !canDeleteAttachment(actor, a) {
		return domain.ErrForbidden
	}
	if err := s.attachments.Delete(ctx, attachmentID); err != nil {
		return err
	}
	if err := s.storage.Delete(ctx, a.StoragePath); err != nil {
		// DB row already gone — return the error so the caller can log.
		return err
	}
	return nil
}

func canDeleteAttachment(actor *entity.User, a *entity.TicketAttachment) bool {
	if actor.Role == entity.RoleAdmin {
		return true
	}
	return a.UploadedBy == actor.ID
}

func sanitiseOriginalFilename(raw string) string {
	// Strip path components — only the last segment is allowed through.
	name := filepath.Base(strings.TrimSpace(raw))
	if name == "." || name == "/" || name == "\\" {
		return ""
	}
	name = safeFilenamePattern.ReplaceAllString(name, "_")
	name = strings.Trim(name, "._-")
	if len(name) > 255 {
		name = name[:255]
	}
	return name
}

// storageDriverName tells the persistence layer which driver produced a row.
// We use a tiny interface assertion so tests can inject a custom driver name
// without making the FileStorage interface chatty.
type driverNamed interface {
	DriverName() string
}

func storageDriverName(fs service.FileStorage) string {
	if dn, ok := fs.(driverNamed); ok {
		return dn.DriverName()
	}
	return "local"
}

func toAttachmentOutput(a *entity.TicketAttachment) AttachmentOutput {
	out := AttachmentOutput{
		ID:               a.ID,
		TicketID:         a.TicketID,
		OriginalFilename: a.OriginalFilename,
		MimeType:         a.MimeType,
		SizeBytes:        a.SizeBytes,
		CreatedAt:        a.CreatedAt,
	}
	if a.Uploader != nil {
		out.UploadedBy = userSummary(a.Uploader)
	} else {
		out.UploadedBy = &UserSummary{ID: a.UploadedBy}
	}
	return out
}
