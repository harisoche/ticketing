DROP INDEX IF EXISTS idx_ticket_histories_actor_id;
DROP INDEX IF EXISTS idx_ticket_histories_ticket_id_created_at;
DROP TABLE IF EXISTS ticket_histories;

ALTER TABLE tickets
    DROP CONSTRAINT IF EXISTS chk_tickets_status;

-- Restore the Phase 2 status check (no 'reopened').
ALTER TABLE tickets
    ADD CONSTRAINT chk_tickets_status
    CHECK (status IN ('open', 'in_progress', 'resolved', 'closed'));

ALTER TABLE tickets
    DROP COLUMN IF EXISTS assigned_at;
