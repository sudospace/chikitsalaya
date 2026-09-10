-- +goose Up
CREATE TABLE practitioners (
    id              BIGSERIAL PRIMARY KEY,
    full_name       TEXT NOT NULL,
    qualification   TEXT NOT NULL DEFAULT '',
    specialization  TEXT NOT NULL DEFAULT '',
    phone           TEXT NOT NULL DEFAULT '',
    email           TEXT NOT NULL DEFAULT '',
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id              BIGSERIAL PRIMARY KEY,
    email           TEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    role            TEXT NOT NULL CHECK (role IN ('master', 'company_admin', 'doctor', 'receptionist')),
    practitioner_id BIGINT REFERENCES practitioners(id) ON DELETE SET NULL,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE patients (
    id                          BIGSERIAL PRIMARY KEY,
    mrn                         TEXT NOT NULL UNIQUE,
    first_name                  TEXT NOT NULL,
    last_name                   TEXT NOT NULL DEFAULT '',
    dob                         DATE,
    sex                         TEXT NOT NULL DEFAULT '',
    phone                       TEXT NOT NULL DEFAULT '',
    email                       TEXT NOT NULL DEFAULT '',
    address                     TEXT NOT NULL DEFAULT '',
    emergency_contact_name      TEXT NOT NULL DEFAULT '',
    emergency_contact_phone     TEXT NOT NULL DEFAULT '',
    blood_group                 TEXT NOT NULL DEFAULT '',
    allergies                   TEXT NOT NULL DEFAULT '',
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_patients_name ON patients (last_name, first_name);
CREATE INDEX idx_patients_phone ON patients (phone);

-- +goose Down
DROP TABLE patients;
DROP TABLE users;
DROP TABLE practitioners;
