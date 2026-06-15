-- Phase 5 — ticket attachments.
--
-- ticket_categories and tickets.priority already exist from Phase 2, so this
-- migration only adds the attachment table.
-- uploaded_by is BIGINT to match users.id.

CREATE TABLE IF NOT EXISTS ticket_attachments (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id         UUID NOT NULL,
    uploaded_by       BIGINT NOT NULL,
    storage_driver    VARCHAR(30) NOT NULL,
    storage_path      TEXT NOT NULL,
    original_filename VARCHAR(255) NOT NULL,
    stored_filename   VARCHAR(255) NOT NULL,
    mime_type         VARCHAR(120) NOT NULL,
    size_bytes        BIGINT NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_ticket_attachments_ticket
        FOREIGN KEY (ticket_id) REFERENCES tickets(id) ON DELETE CASCADE,
    CONSTRAINT fk_ticket_attachments_uploader
        FOREIGN KEY (uploaded_by) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT chk_ticket_attachments_size_positive
        CHECK (size_bytes > 0)
);

CREATE INDEX IF NOT EXISTS idx_ticket_attachments_ticket_id_created_at
    ON ticket_attachments (ticket_id, created_at ASC, id ASC);

CREATE INDEX IF NOT EXISTS idx_ticket_attachments_uploaded_by
    ON ticket_attachments (uploaded_by);
