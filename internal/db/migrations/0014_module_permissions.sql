-- +goose Up
-- Per-user, per-module access: an admin (master creating a company_admin,
-- or a company_admin creating a doctor/receptionist) picks which modules
-- a login can reach and at what level. Master itself is never gated by
-- this table — it's a global role, checked directly in code.
CREATE TABLE user_module_permissions (
    user_id      BIGINT NOT NULL REFERENCES users(id),
    module       TEXT NOT NULL,
    access_level TEXT NOT NULL CHECK (access_level IN ('view', 'edit')),
    PRIMARY KEY (user_id, module)
);

-- Backfill so every existing account keeps exactly the access it already
-- has today — nobody loses access the moment this ships.
INSERT INTO user_module_permissions (user_id, module, access_level)
SELECT u.id, m.module, 'edit'
FROM users u
CROSS JOIN (VALUES
    ('patients'), ('practitioners'), ('appointments'), ('encounters'),
    ('company_settings'), ('integrations'), ('service_units'), ('drugs'),
    ('lookup_values'), ('lab_tests'), ('procedures'), ('users')
) AS m(module)
WHERE u.role = 'company_admin';

INSERT INTO user_module_permissions (user_id, module, access_level)
SELECT u.id, m.module, m.level
FROM users u
CROSS JOIN (VALUES
    ('patients', 'edit'), ('appointments', 'edit'), ('encounters', 'edit'), ('practitioners', 'view')
) AS m(module, level)
WHERE u.role IN ('doctor', 'receptionist');

-- +goose Down
DROP TABLE user_module_permissions;
