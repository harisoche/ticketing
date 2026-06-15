package entity

import (
	"time"

	"github.com/google/uuid"
)

// TicketComment is one author's plain-text reply on a ticket. Author IDs are
// int64 to match users.id (BIGINT) — same adaptation as the rest of the
// project. PASSWORD HASH is never serialised; the `Author` preload is for
// safe public summary fields only.
type TicketComment struct {
	ID        uuid.UUID
	TicketID  uuid.UUID
	AuthorID  int64
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time

	Author *User
}
