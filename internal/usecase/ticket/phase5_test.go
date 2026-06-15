package ticket

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"ticketing-api/internal/domain"
	"ticketing-api/internal/domain/entity"
	"ticketing-api/internal/domain/service"
)

// ---------- fakes ----------

type fakeAttachmentRepo struct {
	mu     sync.Mutex
	byID   map[uuid.UUID]*entity.TicketAttachment
	users  *fakeUserRepo
	failOn string // "" | "Create"
}

func newFakeAttachmentRepo(users *fakeUserRepo) *fakeAttachmentRepo {
	return &fakeAttachmentRepo{byID: map[uuid.UUID]*entity.TicketAttachment{}, users: users}
}

func (r *fakeAttachmentRepo) preload(a *entity.TicketAttachment) {
	if r.users == nil {
		return
	}
	if u, ok := r.users.byID[a.UploadedBy]; ok {
		clone := *u
		a.Uploader = &clone
	}
}

func (r *fakeAttachmentRepo) Create(_ context.Context, a *entity.TicketAttachment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failOn == "Create" {
		return errors.New("induced create failure")
	}
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	a.CreatedAt = time.Now().UTC()
	r.preload(a)
	clone := *a
	r.byID[a.ID] = &clone
	return nil
}

func (r *fakeAttachmentRepo) ListByTicketID(_ context.Context, ticketID uuid.UUID) ([]entity.TicketAttachment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []entity.TicketAttachment{}
	for _, a := range r.byID {
		if a.TicketID == ticketID {
			clone := *a
			r.preload(&clone)
			out = append(out, clone)
		}
	}
	return out, nil
}

func (r *fakeAttachmentRepo) FindByID(_ context.Context, id uuid.UUID) (*entity.TicketAttachment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.byID[id]
	if !ok {
		return nil, domain.ErrAttachmentNotFound
	}
	clone := *a
	r.preload(&clone)
	return &clone, nil
}

func (r *fakeAttachmentRepo) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[id]; !ok {
		return domain.ErrAttachmentNotFound
	}
	delete(r.byID, id)
	return nil
}

// memoryStorage: in-memory FileStorage.
type memoryStorage struct {
	mu       sync.Mutex
	contents map[string][]byte
	failSave bool
	failOpen bool
	delCalls []string
}

func newMemoryStorage() *memoryStorage { return &memoryStorage{contents: map[string][]byte{}} }

func (m *memoryStorage) Save(_ context.Context, in service.SaveFileInput) (*service.StoredFile, error) {
	if m.failSave {
		return nil, errors.New("induced save failure")
	}
	body, _ := io.ReadAll(in.Body)
	name := uuid.NewString() + "." + in.Extension
	m.mu.Lock()
	m.contents[name] = body
	m.mu.Unlock()
	return &service.StoredFile{StoragePath: name, StoredFilename: name}, nil
}

func (m *memoryStorage) Open(_ context.Context, path string) (io.ReadCloser, error) {
	if m.failOpen {
		return nil, errors.New("induced open failure")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	body, ok := m.contents[path]
	if !ok {
		return nil, errors.New("not found")
	}
	return io.NopCloser(bytes.NewReader(body)), nil
}

func (m *memoryStorage) Delete(_ context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.contents, path)
	m.delCalls = append(m.delCalls, path)
	return nil
}

// ---------- harness wrapper ----------

type phase5Harness struct {
	*harness
	atts    *fakeAttachmentRepo
	storage *memoryStorage
	limit   int64
}

func setupPhase5() *phase5Harness {
	h := setup()
	atts := newFakeAttachmentRepo(h.users)
	store := newMemoryStorage()
	const limit = int64(1024) // 1 KiB cap for tests
	h.svc = h.svc.WithPhase5(atts, store, limit)
	return &phase5Harness{harness: h, atts: atts, storage: store, limit: limit}
}

// Helpers ------------------------------------------------------------

// PNG header + IHDR chunk so http.DetectContentType returns image/png.
var pngBytes = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89,
}

// JPEG magic
var jpegBytes = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}

// PDF magic
var pdfBytes = []byte("%PDF-1.4\n%abcd\n")

// =========== category admin tests ===========

// 1. Admin creates a category.
func TestAdminCreateCategory_Success(t *testing.T) {
	h := setupPhase5()
	out, err := h.svc.AdminCreateCategory(context.Background(), h.admin, CreateCategoryInput{
		Name: "Network", Slug: "network",
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if out.Name != "Network" {
		t.Errorf("name mismatch: %q", out.Name)
	}
	if !out.IsActive {
		t.Errorf("expected IsActive true")
	}
}

// 2. Non-admin cannot create a category.
func TestAdminCreateCategory_NonAdminForbidden(t *testing.T) {
	h := setupPhase5()
	_, err := h.svc.AdminCreateCategory(context.Background(), h.agent, CreateCategoryInput{
		Name: "Network", Slug: "network",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	_, err = h.svc.AdminCreateCategory(context.Background(), h.custA, CreateCategoryInput{
		Name: "Network", Slug: "network",
	})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for customer, got %v", err)
	}
}

// 3. Public listing excludes inactive categories.
func TestPublicListing_ExcludesInactive(t *testing.T) {
	h := setupPhase5()
	active, _ := h.svc.AdminCreateCategory(context.Background(), h.admin, CreateCategoryInput{Name: "Active", Slug: "active"})
	hidden, _ := h.svc.AdminCreateCategory(context.Background(), h.admin, CreateCategoryInput{Name: "Hidden", Slug: "hidden"})
	if _, err := h.svc.AdminDeactivateCategory(context.Background(), h.admin, hidden.ID); err != nil {
		t.Fatalf("deactivate failed: %v", err)
	}
	rows, err := h.svc.ListPublicCategories(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var sawActive, sawHidden bool
	for _, r := range rows {
		if r.ID == active.ID {
			sawActive = true
		}
		if r.ID == hidden.ID {
			sawHidden = true
		}
	}
	if !sawActive {
		t.Error("expected active category in public list")
	}
	if sawHidden {
		t.Error("inactive category leaked into public list")
	}
}

// 4. New ticket rejects inactive category.
func TestCreateTicket_RejectsInactiveCategory(t *testing.T) {
	h := setupPhase5()
	created, _ := h.svc.AdminCreateCategory(context.Background(), h.admin, CreateCategoryInput{Name: "Tmp", Slug: "tmp"})
	if _, err := h.svc.AdminDeactivateCategory(context.Background(), h.admin, created.ID); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	_, err := h.svc.Create(context.Background(), h.custA, CreateTicketInput{
		Title:       "Test ticket",
		Description: "Description that is long enough.",
		CategoryID:  created.ID,
		Priority:    entity.TicketPriorityHigh,
	})
	if !errors.Is(err, domain.ErrTicketCategoryNotFound) {
		t.Fatalf("expected ErrTicketCategoryNotFound, got %v", err)
	}
}

// 5. New ticket rejects unsupported priority.
func TestCreateTicket_RejectsBadPriority(t *testing.T) {
	h := setupPhase5()
	_, err := h.svc.Create(context.Background(), h.custA, CreateTicketInput{
		Title: "Test ticket", Description: "long enough description", CategoryID: h.cat.ID,
		Priority: "panic",
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

// 6. Assigned agent updates classification.
func TestClassify_AssignedAgentSucceeds(t *testing.T) {
	h := setupPhase5()
	ticket := mustCreate(t, h.harness, h.custA)
	assign(t, h.harness, h.admin, ticket.ID, h.agent)
	urgent := entity.TicketPriorityUrgent
	out, err := h.svc.ClassifyTicket(context.Background(), h.agent, ticket.ID, ClassifyTicketInput{Priority: &urgent})
	if err != nil {
		t.Fatalf("classify failed: %v", err)
	}
	if out.Priority != urgent {
		t.Errorf("priority not updated, got %q", out.Priority)
	}
}

// 7. Unrelated agent cannot update classification.
func TestClassify_OtherAgentForbidden(t *testing.T) {
	h := setupPhase5()
	ticket := mustCreate(t, h.harness, h.custA)
	assign(t, h.harness, h.admin, ticket.ID, h.agent)
	urgent := entity.TicketPriorityUrgent
	_, err := h.svc.ClassifyTicket(context.Background(), h.agent2, ticket.ID, ClassifyTicketInput{Priority: &urgent})
	// Other agent can't view the ticket at all.
	if !errors.Is(err, domain.ErrTicketNotFound) && !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected forbidden or not-found, got %v", err)
	}
}

// Customer cannot reach the dedicated classification endpoint.
func TestClassify_CustomerForbidden(t *testing.T) {
	h := setupPhase5()
	ticket := mustCreate(t, h.harness, h.custA)
	urgent := entity.TicketPriorityUrgent
	_, err := h.svc.ClassifyTicket(context.Background(), h.custA, ticket.ID, ClassifyTicketInput{Priority: &urgent})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

// Customer can't sneak classification through the generic Update either.
func TestUpdate_CustomerCannotChangeClassification(t *testing.T) {
	h := setupPhase5()
	ticket := mustCreate(t, h.harness, h.custA)
	urgent := entity.TicketPriorityUrgent
	_, err := h.svc.Update(context.Background(), h.custA, ticket.ID, UpdateTicketInput{Priority: &urgent})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

// =========== attachment tests ===========

func addAttachment(t *testing.T, h *phase5Harness, actor *entity.User, ticketID uuid.UUID, body []byte, name string) *AttachmentOutput {
	t.Helper()
	out, err := h.svc.AddAttachment(context.Background(), actor, ticketID, bytes.NewReader(body), name, int64(len(body)))
	if err != nil {
		t.Fatalf("AddAttachment failed: %v", err)
	}
	return out
}

// 8. Accessible actor uploads an allowed file.
func TestAttachment_UploadAllowedTypes(t *testing.T) {
	h := setupPhase5()
	ticket := mustCreate(t, h.harness, h.custA)
	cases := []struct {
		body []byte
		name string
		mime string
	}{
		{pngBytes, "screenshot.png", "image/png"},
		{jpegBytes, "photo.jpg", "image/jpeg"},
		{pdfBytes, "report.pdf", "application/pdf"},
		{[]byte("just plain text content here"), "notes.txt", "text/plain"},
	}
	for _, tc := range cases {
		out := addAttachment(t, h, h.custA, ticket.ID, tc.body, tc.name)
		if out.MimeType != tc.mime {
			t.Errorf("name %q expected mime %q got %q", tc.name, tc.mime, out.MimeType)
		}
		if out.OriginalFilename == "" {
			t.Errorf("name %q produced empty display filename", tc.name)
		}
	}
}

// 9. Oversized file is rejected.
func TestAttachment_RejectsOversize(t *testing.T) {
	h := setupPhase5()
	ticket := mustCreate(t, h.harness, h.custA)
	big := bytes.Repeat([]byte("a"), int(h.limit)+10)
	_, err := h.svc.AddAttachment(context.Background(), h.custA, ticket.ID, bytes.NewReader(big), "big.txt", int64(len(big)))
	if !errors.Is(err, domain.ErrAttachmentTooLarge) {
		t.Fatalf("expected ErrAttachmentTooLarge, got %v", err)
	}
}

// 10. Unsupported MIME type is rejected.
func TestAttachment_RejectsUnsupportedMime(t *testing.T) {
	h := setupPhase5()
	ticket := mustCreate(t, h.harness, h.custA)
	// ZIP magic number triggers application/zip detection.
	zip := []byte{0x50, 0x4b, 0x03, 0x04, 0x14, 0x00, 0x00, 0x00}
	_, err := h.svc.AddAttachment(context.Background(), h.custA, ticket.ID, bytes.NewReader(zip), "archive.zip", int64(len(zip)))
	if !errors.Is(err, domain.ErrAttachmentUnsupported) {
		t.Fatalf("expected ErrAttachmentUnsupported, got %v", err)
	}
}

// 11. Stored filename is generated and not equal to unsafe original filename.
func TestAttachment_StoredFilenameSanitised(t *testing.T) {
	h := setupPhase5()
	ticket := mustCreate(t, h.harness, h.custA)
	out := addAttachment(t, h, h.custA, ticket.ID, pngBytes, "../../etc/passwd.png")

	// Look up the underlying entity to inspect the storage path.
	var stored *entity.TicketAttachment
	for _, a := range h.atts.byID {
		stored = a
		break
	}
	if stored == nil {
		t.Fatal("expected one attachment row")
	}
	if stored.StoredFilename == stored.OriginalFilename {
		t.Errorf("stored filename should differ from original (got %q)", stored.StoredFilename)
	}
	if strings.Contains(stored.StoragePath, "..") || strings.Contains(stored.StoragePath, "etc/passwd") {
		t.Errorf("storage path leaked traversal: %q", stored.StoragePath)
	}
	if strings.Contains(out.OriginalFilename, "/") || strings.Contains(out.OriginalFilename, "..") {
		t.Errorf("display filename not sanitised: %q", out.OriginalFilename)
	}
}

// 12. Database failure after storage save cleans up the file.
func TestAttachment_DBFailureRollsBackStorage(t *testing.T) {
	h := setupPhase5()
	ticket := mustCreate(t, h.harness, h.custA)
	h.atts.failOn = "Create"
	_, err := h.svc.AddAttachment(context.Background(), h.custA, ticket.ID, bytes.NewReader(pngBytes), "screenshot.png", int64(len(pngBytes)))
	if err == nil {
		t.Fatal("expected error when DB insert fails")
	}
	// Storage should have been called Save then Delete exactly once.
	if len(h.storage.delCalls) != 1 {
		t.Fatalf("expected one delete call after rollback, got %d", len(h.storage.delCalls))
	}
	if len(h.storage.contents) != 0 {
		t.Fatalf("expected stored file removed, %d still present", len(h.storage.contents))
	}
}

// 13. Unrelated user cannot list or download attachments.
func TestAttachment_UnrelatedUserDenied(t *testing.T) {
	h := setupPhase5()
	ticket := mustCreate(t, h.harness, h.custA)
	att := addAttachment(t, h, h.custA, ticket.ID, pngBytes, "x.png")

	_, err := h.svc.ListAttachments(context.Background(), h.custB, ticket.ID)
	if !errors.Is(err, domain.ErrTicketNotFound) {
		t.Errorf("expected list denied, got %v", err)
	}
	_, _, err = h.svc.DownloadAttachment(context.Background(), h.custB, ticket.ID, att.ID)
	if !errors.Is(err, domain.ErrTicketNotFound) {
		t.Errorf("expected download denied, got %v", err)
	}
}

// 14. Attachment ID must belong to path ticket.
func TestAttachment_PathTicketMismatch(t *testing.T) {
	h := setupPhase5()
	t1 := mustCreate(t, h.harness, h.custA)
	t2 := mustCreate(t, h.harness, h.custA)
	att := addAttachment(t, h, h.custA, t1.ID, pngBytes, "x.png")

	_, _, err := h.svc.DownloadAttachment(context.Background(), h.custA, t2.ID, att.ID)
	if !errors.Is(err, domain.ErrAttachmentNotFound) {
		t.Errorf("expected ErrAttachmentNotFound, got %v", err)
	}
	err = h.svc.DeleteAttachment(context.Background(), h.custA, t2.ID, att.ID)
	if !errors.Is(err, domain.ErrAttachmentNotFound) {
		t.Errorf("expected ErrAttachmentNotFound, got %v", err)
	}
}

// 15. Uploader deletes own attachment.
func TestAttachment_UploaderDeletesOwn(t *testing.T) {
	h := setupPhase5()
	ticket := mustCreate(t, h.harness, h.custA)
	att := addAttachment(t, h, h.custA, ticket.ID, pngBytes, "x.png")
	if err := h.svc.DeleteAttachment(context.Background(), h.custA, ticket.ID, att.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if len(h.atts.byID) != 0 {
		t.Error("expected attachment removed from DB")
	}
	if len(h.storage.contents) != 0 {
		t.Error("expected stored file removed")
	}
}

// 16. Admin deletes another user's attachment.
func TestAttachment_AdminDeletesAny(t *testing.T) {
	h := setupPhase5()
	ticket := mustCreate(t, h.harness, h.custA)
	att := addAttachment(t, h, h.custA, ticket.ID, pngBytes, "x.png")
	if err := h.svc.DeleteAttachment(context.Background(), h.admin, ticket.ID, att.ID); err != nil {
		t.Fatalf("admin delete failed: %v", err)
	}
}

// 17. Normal user cannot delete another user's attachment.
func TestAttachment_NonAuthorCannotDelete(t *testing.T) {
	h := setupPhase5()
	ticket := mustCreate(t, h.harness, h.custA)
	assign(t, h.harness, h.admin, ticket.ID, h.agent)
	att := addAttachment(t, h, h.custA, ticket.ID, pngBytes, "x.png")
	// Agent can access ticket but is not the uploader.
	err := h.svc.DeleteAttachment(context.Background(), h.agent, ticket.ID, att.ID)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

// Download returns the stored bytes when the actor can view the ticket.
func TestAttachment_DownloadReturnsBody(t *testing.T) {
	h := setupPhase5()
	ticket := mustCreate(t, h.harness, h.custA)
	att := addAttachment(t, h, h.custA, ticket.ID, pdfBytes, "report.pdf")
	meta, rc, err := h.svc.DownloadAttachment(context.Background(), h.custA, ticket.ID, att.ID)
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	defer rc.Close()
	body, _ := io.ReadAll(rc)
	if !bytes.Equal(body, pdfBytes) {
		t.Errorf("body mismatch")
	}
	if meta.MimeType != "application/pdf" {
		t.Errorf("mime mismatch: %q", meta.MimeType)
	}
}
