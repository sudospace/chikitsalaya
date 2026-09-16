-- +goose Up
-- Widen the module allowlist for a 4th integrations module: per-clinic
-- file-storage backend config (local/gdrive/s3), reusing the same
-- provider+JSONB-config pattern already used for sms/email/payment.
-- The constraint's name is still "company_integrations_..." from before
-- the table itself was renamed to "integrations" (0020_single_tenant.sql).
ALTER TABLE integrations DROP CONSTRAINT company_integrations_module_check;
ALTER TABLE integrations ADD CONSTRAINT integrations_module_check
    CHECK (module IN ('sms', 'email', 'payment', 'storage'));

-- +goose Down
ALTER TABLE integrations DROP CONSTRAINT integrations_module_check;
ALTER TABLE integrations ADD CONSTRAINT company_integrations_module_check
    CHECK (module IN ('sms', 'email', 'payment'));
