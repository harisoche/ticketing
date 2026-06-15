ALTER TABLE users
    ADD COLUMN IF NOT EXISTS role VARCHAR(30) NOT NULL DEFAULT 'customer';

ALTER TABLE users
    ADD CONSTRAINT chk_users_role
    CHECK (role IN ('customer', 'agent', 'admin'));

CREATE INDEX IF NOT EXISTS idx_users_role ON users (role);
