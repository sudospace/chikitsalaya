-- +goose Up
CREATE TABLE appointments (
    id                BIGSERIAL PRIMARY KEY,
    patient_id        BIGINT NOT NULL REFERENCES patients(id),
    practitioner_id   BIGINT NOT NULL REFERENCES practitioners(id),
    company_id        BIGINT NOT NULL REFERENCES companies(id),
    service_unit_id   BIGINT REFERENCES service_units(id) ON DELETE SET NULL,
    scheduled_at      TIMESTAMPTZ NOT NULL,
    duration_minutes  INT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'scheduled'
                        CHECK (status IN ('requested', 'scheduled', 'checked_in', 'completed', 'cancelled', 'no_show')),
    source            TEXT NOT NULL DEFAULT 'staff' CHECK (source IN ('staff', 'online')),
    appointment_type  TEXT NOT NULL DEFAULT '',
    reason_for_visit  TEXT NOT NULL DEFAULT '',
    fee_amount        NUMERIC,
    fee_paid          BOOLEAN NOT NULL DEFAULT FALSE,
    created_by        BIGINT NOT NULL REFERENCES users(id),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- v1 overlap prevention is application-level (see internal/opd/appointment.Store.Create),
-- run inside a transaction. This index supports that check and the day-view query;
-- a GIST exclusion constraint on (practitioner_id, tstzrange(...)) would make the
-- invariant DB-enforced but is deferred per the project plan.
CREATE INDEX idx_appointments_practitioner_time ON appointments (practitioner_id, scheduled_at);
CREATE INDEX idx_appointments_patient ON appointments (patient_id);

-- +goose Down
DROP TABLE appointments;
