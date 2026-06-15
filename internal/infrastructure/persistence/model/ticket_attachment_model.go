package model

import (
	"time"

	"github.com/google/uuid"

	"ticketing-api/internal/domain/entity"
)

// TicketAttachmentModel is the GORM row for ticket_attachments.
type TicketAttachmentModel struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey;column:id"`
	TicketID         uuid.UUID `gorm:"type:uuid;column:ticket_id;not null"`
	UploadedBy       int64     `gorm:"column:uploaded_by;not null"`
	StorageDriver    string    `gorm:"column:storage_driver;size:30;not null"`
	StoragePath      string    `gorm:"column:storage_path;type:text;not null"`
	OriginalFilename string    `gorm:"column:original_filename;size:255;not null"`
	StoredFilename   string    `gorm:"column:stored_filename;size:255;not null"`
	MimeType         string    `gorm:"column:mime_type;size:120;not null"`
	SizeBytes        int64     `gorm:"column:size_bytes;not null"`
	CreatedAt        time.Time `gorm:"column:created_at;not null;autoCreateTime"`

	Uploader *UserModel `gorm:"foreignKey:UploadedBy;references:ID"`
}

func (TicketAttachmentModel) TableName() string { return "ticket_attachments" }

func TicketAttachmentModelFromEntity(a *entity.TicketAttachment) *TicketAttachmentModel {
	return &TicketAttachmentModel{
		ID:               a.ID,
		TicketID:         a.TicketID,
		UploadedBy:       a.UploadedBy,
		StorageDriver:    a.StorageDriver,
		StoragePath:      a.StoragePath,
		OriginalFilename: a.OriginalFilename,
		StoredFilename:   a.StoredFilename,
		MimeType:         a.MimeType,
		SizeBytes:        a.SizeBytes,
		CreatedAt:        a.CreatedAt,
	}
}

func (m *TicketAttachmentModel) ToEntity() *entity.TicketAttachment {
	out := &entity.TicketAttachment{
		ID:               m.ID,
		TicketID:         m.TicketID,
		UploadedBy:       m.UploadedBy,
		StorageDriver:    m.StorageDriver,
		StoragePath:      m.StoragePath,
		OriginalFilename: m.OriginalFilename,
		StoredFilename:   m.StoredFilename,
		MimeType:         m.MimeType,
		SizeBytes:        m.SizeBytes,
		CreatedAt:        m.CreatedAt,
	}
	if m.Uploader != nil {
		out.Uploader = m.Uploader.ToEntity()
	}
	return out
}
