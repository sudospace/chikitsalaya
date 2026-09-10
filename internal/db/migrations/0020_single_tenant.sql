-- +goose Up
-- Chikitsalaya moves from multi-tenant (many clinics per deployment) to
-- single-tenant (one deployment, one clinic) — this purges company_id from
-- every table that had it and collapses the companies table into a single-
-- row clinic_settings table. Forward-only: the dev DB is wiped and reseeded
-- alongside this migration, there's no multi-company data to preserve.
ALTER TABLE companies RENAME TO clinic_settings;
ALTER TABLE clinic_settings DROP COLUMN is_active;
ALTER TABLE clinic_settings DROP COLUMN name;
ALTER TABLE clinic_settings RENAME COLUMN display_name TO name;

ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('admin', 'doctor', 'receptionist'));
UPDATE users SET role = 'admin' WHERE role IN ('master', 'company_admin');

ALTER TABLE users               DROP COLUMN company_id;
ALTER TABLE service_units       DROP COLUMN company_id;
ALTER TABLE practitioners       DROP COLUMN company_id;
ALTER TABLE appointments        DROP COLUMN company_id;
ALTER TABLE encounters          DROP COLUMN company_id;
ALTER TABLE drugs               DROP COLUMN company_id;
ALTER TABLE lab_test_templates  DROP COLUMN company_id;
ALTER TABLE procedure_templates DROP COLUMN company_id;

ALTER TABLE lookup_values DROP COLUMN company_id;
ALTER TABLE lookup_values ADD CONSTRAINT lookup_values_category_value_key UNIQUE (category, value);

ALTER TABLE company_integrations DROP COLUMN company_id;
ALTER TABLE company_integrations RENAME TO integrations;
ALTER TABLE integrations ADD CONSTRAINT integrations_module_key UNIQUE (module);

-- +goose Down
-- One-way purge tied to a wipe-and-reseed decision; restore from a
-- pre-0020 backup instead of running this Down if multi-tenant is ever
-- needed again.
SELECT 1;
