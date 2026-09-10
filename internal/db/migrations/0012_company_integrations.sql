-- +goose Up
-- Per-company third-party integration config: SMS, email, and payment are
-- separate "modules" a company_admin configures independently. config is a
-- flexible key/value bag (JSONB) since each module/provider needs
-- different fields — the fixed columns (module, provider, is_active) are
-- what the app actually branches on.
CREATE TABLE company_integrations (
    id          BIGSERIAL PRIMARY KEY,
    company_id  BIGINT NOT NULL REFERENCES companies(id),
    module      TEXT NOT NULL CHECK (module IN ('sms', 'email', 'payment')),
    provider    TEXT NOT NULL DEFAULT 'none',
    config      JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_active   BOOLEAN NOT NULL DEFAULT false,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (company_id, module)
);

-- +goose Down
DROP TABLE company_integrations;
