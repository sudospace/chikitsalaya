-- +goose Up
-- No backfill needed: practitioners can only be created after a company
-- exists (via onboarding), so this table is always empty when this
-- migration runs on a fresh install.
ALTER TABLE practitioners ADD COLUMN company_id BIGINT NOT NULL REFERENCES companies(id);

CREATE TABLE practitioner_schedules (
    id                     BIGSERIAL PRIMARY KEY,
    practitioner_id        BIGINT NOT NULL REFERENCES practitioners(id) ON DELETE CASCADE,
    day_of_week            SMALLINT NOT NULL CHECK (day_of_week BETWEEN 0 AND 6), -- 0=Sunday .. 6=Saturday, matches Go's time.Weekday
    start_time             TIME NOT NULL,
    end_time               TIME NOT NULL,
    slot_duration_minutes  INT NOT NULL DEFAULT 15,
    service_unit_id        BIGINT REFERENCES service_units(id) ON DELETE SET NULL,
    is_active              BOOLEAN NOT NULL DEFAULT TRUE,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (end_time > start_time)
);

CREATE INDEX idx_practitioner_schedules_practitioner ON practitioner_schedules (practitioner_id);

-- +goose Down
DROP TABLE practitioner_schedules;
ALTER TABLE practitioners DROP COLUMN company_id;
