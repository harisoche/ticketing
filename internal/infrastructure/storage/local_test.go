package storage

import (
	"context"
	"io"
	"strings"
	"testing"

	"ticketing-api/internal/domain/service"
)

func TestLocalStorage_SaveOpenDelete(t *testing.T) {
	root := t.TempDir()
	s, err := NewLocalFileStorage(root)
	if err != nil {
		t.Fatalf("NewLocalFileStorage: %v", err)
	}
	ctx := context.Background()
	stored, err := s.Save(ctx, service.SaveFileInput{
		Body:      strings.NewReader("hello, attachment"),
		MimeType:  "text/plain",
		Extension: "txt",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if stored.StoredFilename == "" {
		t.Fatal("expected stored filename")
	}
	if !strings.HasSuffix(stored.StoragePath, ".txt") {
		t.Errorf("storage path should keep extension, got %q", stored.StoragePath)
	}

	rc, err := s.Open(ctx, stored.StoragePath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(b) != "hello, attachment" {
		t.Errorf("body mismatch: %q", b)
	}

	if err := s.Delete(ctx, stored.StoragePath); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

func TestLocalStorage_RejectsTraversal(t *testing.T) {
	root := t.TempDir()
	s, err := NewLocalFileStorage(root)
	if err != nil {
		t.Fatalf("NewLocalFileStorage: %v", err)
	}
	ctx := context.Background()
	for _, bad := range []string{"../escape.txt", "/etc/passwd", "..", ""} {
		if _, err := s.Open(ctx, bad); err == nil {
			t.Errorf("Open(%q) should fail", bad)
		}
		if err := s.Delete(ctx, bad); err == nil {
			t.Errorf("Delete(%q) should fail", bad)
		}
	}
}

func TestLocalStorage_RejectsBadExtension(t *testing.T) {
	root := t.TempDir()
	s, err := NewLocalFileStorage(root)
	if err != nil {
		t.Fatalf("NewLocalFileStorage: %v", err)
	}
	_, err = s.Save(context.Background(), service.SaveFileInput{
		Body:      strings.NewReader("data"),
		MimeType:  "text/plain",
		Extension: "../sneaky",
	})
	if err == nil {
		t.Fatal("expected Save to reject extension with slash")
	}
}
