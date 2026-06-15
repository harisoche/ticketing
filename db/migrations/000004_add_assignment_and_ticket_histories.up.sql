-- Phase 3 — assignment timestamp, workflow audit history, expanded status enum.
--
-- Notes:
--   * actor_id and assignee FKs are BIGINT to match users.id in this repo
--     (the spec template assumes UUID — we adapt to BIGINT, as we did for
--     tickets.created_by / tickets.assigned_to in Phase 2).
--   * Phase 2 already created chk_tickets_status with 4 values. We drop and
--     recreate it to add 'reopened'.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Track when the current assignment was made.
ALTER TABLE tickets
    ADD COLUMN IF NOT EXISTS assigned_at TIMESTAMPTZ NULL;

-- Replace the Phase 2 status CHECK so it permits 'reopened'.
ALTER TABLE tickets
    DROP CONSTRAINT IF EXISTS chk_tickets_status;

ALTER TABLE tickets
    ADD CONSTRAINT chk_tickets_status
    CHECK (status IN ('open', 'in_progress', 'resolved', 'closed', 'reopened'));

CREATE TABLE IF NOT EXISTS ticket_histories (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id       UUID NOT NULL,
    actor_id        BIGINT NOT NULL,
    action          VARCHAR(30) NOT NULL,
    old_status      VARCHAR(30) NULL,
    new_status      VARCHAR(30) NULL,
    old_assignee_id BIGINT NULL,
    new_assignee_id BIGINT NULL,
    note            TEXT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_ticket_histories_ticket
        FOREIGN KEY (ticket_id) REFERENCES tickets(id) ON DELETE CASCADE,
    CONSTRAINT fk_ticket_histories_actor
        FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT fk_ticket_histories_old_assignee
        FOREIGN KEY (old_assignee_id) REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT fk_ticket_histories_new_assignee
        FOREIGN KEY (new_assignee_id) REFERENCES users(id) ON DELETE SET NULL,
    CONSTRAINT chk_ticket_histories_action
        CHECK (action IN ('created', 'assigned', 'reassigned', 'unassigned', 'status_changed'))
);

CREATE INDEX IF NOT EXISTS idx_ticket_histories_ticket_id_created_at
    ON ticket_histories (ticket_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_ticket_histories_actor_id
    ON ticket_histories (actor_id);
