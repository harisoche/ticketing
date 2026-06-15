-- Phase 6 — list/search indexes.
--
-- Composite indexes are partial (WHERE deleted_at IS NULL) to match how the
-- application queries tickets. The trigram indexes are optional: comment
-- out the pg_trgm block if the target environment forbids CREATE EXTENSION.

-- Composite covering indexes for the most common filter shapes.
CREATE INDEX IF NOT EXISTS idx_tickets_status_priority_created_at
    ON tickets (status, priority, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_tickets_category_id_created_at
    ON tickets (category_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_tickets_created_by_created_at
    ON tickets (created_by, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_tickets_assigned_to_created_at
    ON tickets (assigned_to, created_at DESC)
    WHERE deleted_at IS NULL;

-- Optional trigram support for ILIKE '%keyword%' acceleration.
-- pg_trgm is "trusted" since PG13, so a non-superuser that owns the
-- database can create it. If your environment forbids this, remove the
-- following block — application search continues to work via ILIKE.
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_tickets_title_trgm
    ON tickets USING gin (title gin_trgm_ops)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_tickets_description_trgm
    ON tickets USING gin (description gin_trgm_ops)
    WHERE deleted_at IS NULL;
