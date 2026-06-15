-- Phase 2 — ticket categories and tickets.
-- Note: users.id is BIGINT in this project, so created_by / assigned_to are BIGINT
-- rather than UUID as in the generic Phase 2 spec template.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE SEQUENCE IF NOT EXISTS ticket_code_seq
    START WITH 1
    INCREMENT BY 1
    MINVALUE 1
    NO MAXVALUE
    CACHE 1;

CREATE TABLE IF NOT EXISTS ticket_categories (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(100) NOT NULL,
    slug        VARCHAR(120) NOT NULL UNIQUE,
    description TEXT NULL,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_ticket_categories_name_active
    ON ticket_categories (LOWER(name))
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_ticket_categories_active
    ON ticket_categories (is_active)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS tickets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        VARCHAR(20) NOT NULL UNIQUE DEFAULT ('TKT-' || LPAD(nextval('ticket_code_seq')::TEXT, 8, '0')),
    title       VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    status      VARCHAR(30) NOT NULL DEFAULT 'open',
    priority    VARCHAR(20) NOT NULL DEFAULT 'medium',
    category_id UUID NOT NULL,
    created_by  BIGINT NOT NULL,
    assigned_to BIGINT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at  TIMESTAMPTZ NULL,

    CONSTRAINT fk_tickets_category
        FOREIGN KEY (category_id) REFERENCES ticket_categories(id),
    CONSTRAINT fk_tickets_created_by
        FOREIGN KEY (created_by) REFERENCES users(id),
    CONSTRAINT fk_tickets_assigned_to
        FOREIGN KEY (assigned_to) REFERENCES users(id),
    CONSTRAINT chk_tickets_status
        CHECK (status IN ('open', 'in_progress', 'resolved', 'closed')),
    CONSTRAINT chk_tickets_priority
        CHECK (priority IN ('low', 'medium', 'high', 'urgent'))
);

CREATE INDEX IF NOT EXISTS idx_tickets_created_by
    ON tickets (created_by)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_tickets_assigned_to
    ON tickets (assigned_to)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_tickets_category_id
    ON tickets (category_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_tickets_status
    ON tickets (status)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_tickets_priority
    ON tickets (priority)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_tickets_created_at
    ON tickets (created_at DESC)
    WHERE deleted_at IS NULL;

INSERT INTO ticket_categories (name, slug, description)
VALUES
    ('Technical Issue', 'technical-issue', 'Problems related to application errors, bugs, or technical access.'),
    ('Account Issue',   'account-issue',   'Problems related to user accounts, profiles, or access.'),
    ('Billing Issue',   'billing-issue',   'Questions or problems related to payments and billing.'),
    ('General Inquiry', 'general-inquiry', 'General questions that do not match another category.')
ON CONFLICT (slug) DO NOTHING;
