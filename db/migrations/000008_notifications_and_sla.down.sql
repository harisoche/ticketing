DROP INDEX IF EXISTS idx_tickets_resolution_due_at;
DROP INDEX IF EXISTS idx_notifications_recipient_id_unread;
DROP INDEX IF EXISTS idx_notifications_recipient_id_created_at;
DROP TABLE IF EXISTS notifications;

ALTER TABLE tickets
    DROP COLUMN IF EXISTS closed_at,
    DROP COLUMN IF EXISTS resolved_at,
    DROP COLUMN IF EXISTS first_responded_at,
    DROP COLUMN IF EXISTS resolution_due_at,
    DROP COLUMN IF EXISTS response_due_at;

DROP TABLE IF EXISTS sla_policies;
