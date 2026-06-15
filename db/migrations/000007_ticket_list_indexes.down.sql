DROP INDEX IF EXISTS idx_tickets_description_trgm;
DROP INDEX IF EXISTS idx_tickets_title_trgm;
DROP INDEX IF EXISTS idx_tickets_assigned_to_created_at;
DROP INDEX IF EXISTS idx_tickets_created_by_created_at;
DROP INDEX IF EXISTS idx_tickets_category_id_created_at;
DROP INDEX IF EXISTS idx_tickets_status_priority_created_at;

-- Note: we do NOT drop the pg_trgm extension here; other tables may use it.
