-- +goose Up
CREATE TABLE companies (
    id                BIGSERIAL PRIMARY KEY,
    name              TEXT NOT NULL UNIQUE,
    display_name      TEXT NOT NULL DEFAULT '',
    address           TEXT NOT NULL DEFAULT '',
    phone             TEXT NOT NULL DEFAULT '',
    email             TEXT NOT NULL DEFAULT '',
    registration_no   TEXT NOT NULL DEFAULT '',
    logo_path         TEXT NOT NULL DEFAULT '',
    letterhead_html   TEXT NOT NULL DEFAULT '',
    footer_text       TEXT NOT NULL DEFAULT '',
    is_active         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- No auto-seeded company: the master creates the first one through the
-- onboarding wizard. Every request resolves "which company" from the
-- acting user's own company_id, so there's no need for a global default.
ALTER TABLE users ADD COLUMN company_id BIGINT REFERENCES companies(id);
ALTER TABLE users ADD COLUMN onboarded_at TIMESTAMPTZ;

CREATE TABLE service_units (
    id          BIGSERIAL PRIMARY KEY,
    company_id  BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    unit_type   TEXT NOT NULL CHECK (unit_type IN ('consultation_room', 'procedure_room', 'lab')),
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_service_units_company ON service_units (company_id);

-- +goose Down
DROP TABLE service_units;
ALTER TABLE users DROP COLUMN onboarded_at;
ALTER TABLE users DROP COLUMN company_id;
DROP TABLE companies;
