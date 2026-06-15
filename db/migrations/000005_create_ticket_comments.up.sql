-- Phase 4 — ticket comments.
-- author_id is BIGINT to match users.id (see Phase 2/3 inspection notes).

CREATE TABLE IF NOT EXISTS ticket_comments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id  UUID NOT NULL,
    author_id  BIGINT NOT NULL,
    body       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_ticket_comments_ticket
        FOREIGN KEY (ticket_id) REFERENCES tickets(id) ON DELETE CASCADE,
    CONSTRAINT fk_ticket_comments_author
        FOREIGN KEY (author_id) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT chk_ticket_comments_body_not_blank
        CHECK (BTRIM(body) <> '')
);

CREATE INDEX IF NOT EXISTS idx_ticket_comments_ticket_id_created_at
    ON ticket_comments (ticket_id, created_at ASC, id ASC);

CREATE INDEX IF NOT EXISTS idx_ticket_comments_author_id
    ON ticket_comments (author_id);
