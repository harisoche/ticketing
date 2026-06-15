package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"ticketing-api/internal/domain/service"
)

// DriverLocal is the value persisted in ticket_attachments.storage_driver
// so future migrations can identify rows produced by this backend.
const DriverLocal = "local"

// LocalFileStorage stores files under a root directory. Paths returned from
// Save are RELATIVE to the root; absolute filesystem paths never leave this
// package, and Open/Delete refuse anything that escapes the root.
type LocalFileStorage struct {
	root string
}

// NewLocalFileStorage prepares the root directory and returns a storage
// instance. Restrictive permissions (0700) keep the directory unreadable
// to other users on the box.
func NewLocalFileStorage(root string) (*LocalFileStorage, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absRoot, 0o700); err != nil {
		return nil, err
	}
	return &LocalFileStorage{root: absRoot}, nil
}

func (s *LocalFileStorage) Save(ctx context.Context, in service.SaveFileInput) (*service.StoredFile, error) {
	ext := strings.TrimSpace(in.Extension)
	if ext == "" {
		return nil, errors.New("storage: empty extension")
	}
	// Defence-in-depth: extension must be alphanumeric only.
	for _, r := range ext {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') {
			return nil, fmt.Errorf("storage: invalid extension %q", in.Extension)
		}
	}

	storedName := uuid.NewString() + "." + strings.ToLower(ext)
	rel := storedName
	abs := filepath.Join(s.root, rel)

	// O_EXCL prevents a stray collision with an existing file.
	f, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(f, in.Body); err != nil {
		_ = f.Close()
		_ = os.Remove(abs)
		return nil, err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(abs)
		return nil, err
	}
	return &service.StoredFile{StoragePath: rel, StoredFilename: storedName}, nil
}

func (s *LocalFileStorage) Open(ctx context.Context, storagePath string) (io.ReadCloser, error) {
	abs, err := s.resolveSafe(storagePath)
	if err != nil {
		return nil, err
	}
	return os.Open(abs)
}

func (s *LocalFileStorage) Delete(ctx context.Context, storagePath string) error {
	abs, err := s.resolveSafe(storagePath)
	if err != nil {
		return err
	}
	if err := os.Remove(abs); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// resolveSafe joins root + rel and refuses anything that escapes the root.
// This is the only path used to touch the filesystem.
func (s *LocalFileStorage) resolveSafe(rel string) (string, error) {
	if rel == "" {
		return "", errors.New("storage: empty path")
	}
	// Disallow absolute or traversal patterns up front.
	cleaned := filepath.Clean(rel)
	if strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) || cleaned == ".." || filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("storage: illegal path %q", rel)
	}
	abs := filepath.Join(s.root, cleaned)
	// Defensive: even after Clean, ensure abs is inside the root.
	rels, err := filepath.Rel(s.root, abs)
	if err != nil || strings.HasPrefix(rels, "..") {
		return "", fmt.Errorf("storage: path escapes root: %q", rel)
	}
	return abs, nil
}
