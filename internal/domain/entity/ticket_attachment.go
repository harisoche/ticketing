package entity

import (
	"time"

	"github.com/google/uuid"
)

// TicketAttachment is one uploaded file attached to a ticket. uploaded_by is
// int64 to match users.id (BIGINT).
//
// StorageDriver and StoragePath are persistence details — handlers must
// never serialise them to JSON.
type TicketAttachment struct {
	ID               uuid.UUID
	TicketID         uuid.UUID
	UploadedBy       int64
	StorageDriver    string
	StoragePath      string
	OriginalFilename string
	StoredFilename   string
	MimeType         string
	SizeBytes        int64
	CreatedAt        time.Time

	Uploader *User
}
