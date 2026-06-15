package service

import (
	"context"
	"io"
)

// SaveFileInput is the payload Save() reads from. Body is consumed exactly once.
type SaveFileInput struct {
	// Body is the file content. The storage implementation reads it until
	// EOF and is responsible for size accounting if needed.
	Body io.Reader

	// SizeHint, if positive, is the expected number of bytes. Implementations
	// may use it to short-circuit or to size buffers.
	SizeHint int64

	// MimeType is the detected MIME type (e.g. "image/png"). The storage
	// implementation uses it (or the Extension field) to pick a file
	// extension for the stored filename.
	MimeType string

	// Extension is the file extension to append to the stored filename,
	// without the leading dot (e.g. "png"). Storage must accept it as-is.
	Extension string
}

// StoredFile is the result of a successful Save. StoragePath is the opaque
// key the storage backend uses to retrieve the file later — handlers must
// never expose it.
type StoredFile struct {
	StoragePath    string
	StoredFilename string
}

// FileStorage is the abstraction the use case talks to. A future
// S3-compatible implementation can replace LocalFileStorage without touching
// the use-case layer.
type FileStorage interface {
	Save(ctx context.Context, input SaveFileInput) (*StoredFile, error)
	Open(ctx context.Context, storagePath string) (io.ReadCloser, error)
	Delete(ctx context.Context, storagePath string) error
}
