-- Phase 7 — SLA policies, ticket SLA columns, notifications.
--
-- recipient_id is BIGINT to match users.id (same adaptation since Phase 2).

CREATE TABLE IF NOT EXISTS sla_policies (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    priority           VARCHAR(20) NOT NULL,
    response_minutes   INT NOT NULL,
    resolution_minutes INT NOT NULL,
    is_active          BOOLEAN NOT NULL DEFAULT TRUE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT sla_policies_priority_unique UNIQUE (priority),
    CONSTRAINT chk_sla_policies_priority    CHECK (priority IN ('low', 'medium', 'high', 'urgent')),
    CONSTRAINT chk_sla_policies_response    CHECK (response_minutes > 0),
    CONSTRAINT chk_sla_policies_resolution  CHECK (resolution_minutes > 0)
);

INSERT INTO sla_policies (priority, response_minutes, resolution_minutes)
VALUES
    ('low',    480, 2880),
    ('medium', 240, 1440),
    ('high',    60,  480),
    ('urgent',  15,  120)
ON CONFLICT (priority) DO NOTHING;

ALTER TABLE tickets
    ADD COLUMN IF NOT EXISTS response_due_at    TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS resolution_due_at  TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS first_responded_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS resolved_at        TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS closed_at          TIMESTAMPTZ NULL;

-- Backfill SLA due times for existing development tickets based on the seeded
-- policy and the row's created_at. Rows already populated are skipped.
UPDATE tickets t
SET response_due_at   = t.created_at + (p.response_minutes   || ' minutes')::INTERVAL,
    resolution_due_at = t.created_at + (p.resolution_minutes || ' minutes')::INTERVAL
FROM sla_policies p
WHERE p.priority = t.priority
  AND p.is_active = TRUE
  AND (t.response_due_at IS NULL OR t.resolution_due_at IS NULL);

CREATE TABLE IF NOT EXISTS notifications (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    recipient_id BIGINT NOT NULL,
    ticket_id    UUID NULL,
    type         VARCHAR(50) NOT NULL,
    title        VARCHAR(180) NOT NULL,
    message      TEXT NOT NULL,
    read_at      TIMESTAMPTZ NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_notifications_recipient
        FOREIGN KEY (recipient_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT fk_notifications_ticket
        FOREIGN KEY (ticket_id) REFERENCES tickets(id) ON DELETE CASCADE,
    CONSTRAINT chk_notifications_type CHECK (
        type IN ('ticket_assigned', 'ticket_reassigned', 'ticket_status_changed', 'ticket_commented')
    )
);

CREATE INDEX IF NOT EXISTS idx_notifications_recipient_id_created_at
    ON notifications (recipient_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_notifications_recipient_id_unread
    ON notifications (recipient_id)
    WHERE read_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_tickets_resolution_due_at
    ON tickets (resolution_due_at)
    WHERE deleted_at IS NULL;
