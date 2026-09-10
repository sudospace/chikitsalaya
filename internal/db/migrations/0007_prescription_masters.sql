-- +goose Up
-- Per-company catalogs: each company_admin builds their own during
-- onboarding (drugs/lab tests/procedures) or gets sensible dosage/duration
-- defaults auto-seeded when their company is created (see
-- internal/opd/company.Store.CreateCompany).
CREATE TABLE lookup_values (
    id          BIGSERIAL PRIMARY KEY,
    company_id  BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    category    TEXT NOT NULL,
    value       TEXT NOT NULL,
    sort_order  INT NOT NULL DEFAULT 0,
    UNIQUE (company_id, category, value)
);

CREATE TABLE drugs (
    id             BIGSERIAL PRIMARY KEY,
    company_id     BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    name           TEXT NOT NULL,
    generic_name   TEXT NOT NULL DEFAULT '',
    form           TEXT NOT NULL DEFAULT '',
    strength       TEXT NOT NULL DEFAULT '',
    unit           TEXT NOT NULL DEFAULT '',
    stock_qty      NUMERIC NOT NULL DEFAULT 0,
    reorder_level  NUMERIC,
    is_active      BOOLEAN NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_drugs_company ON drugs (company_id);

CREATE TABLE stock_movements (
    id            BIGSERIAL PRIMARY KEY,
    drug_id       BIGINT NOT NULL REFERENCES drugs(id) ON DELETE CASCADE,
    change_qty    NUMERIC NOT NULL,
    reason        TEXT NOT NULL CHECK (reason IN ('opening', 'purchase', 'dispense', 'adjustment')),
    reference_id  BIGINT,
    created_by    BIGINT NOT NULL REFERENCES users(id),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_stock_movements_drug ON stock_movements (drug_id);

CREATE TABLE lab_test_templates (
    id           BIGSERIAL PRIMARY KEY,
    company_id   BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    department   TEXT NOT NULL DEFAULT '',
    price        NUMERIC,
    sample_type  TEXT NOT NULL DEFAULT '',
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_lab_test_templates_company ON lab_test_templates (company_id);

CREATE TABLE procedure_templates (
    id           BIGSERIAL PRIMARY KEY,
    company_id   BIGINT NOT NULL REFERENCES companies(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    department   TEXT NOT NULL DEFAULT '',
    price        NUMERIC,
    description  TEXT NOT NULL DEFAULT '',
    is_active    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_procedure_templates_company ON procedure_templates (company_id);

-- +goose Down
DROP TABLE procedure_templates;
DROP TABLE lab_test_templates;
DROP TABLE stock_movements;
DROP TABLE drugs;
DROP TABLE lookup_values;
