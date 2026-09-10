-- +goose Up
CREATE TABLE encounters (
    id                          BIGSERIAL PRIMARY KEY,
    appointment_id              BIGINT REFERENCES appointments(id),
    patient_id                  BIGINT NOT NULL REFERENCES patients(id),
    practitioner_id             BIGINT NOT NULL REFERENCES practitioners(id),
    company_id                  BIGINT NOT NULL REFERENCES companies(id),
    chief_complaint             TEXT NOT NULL DEFAULT '',
    history_of_present_illness  TEXT NOT NULL DEFAULT '',
    examination_notes           TEXT NOT NULL DEFAULT '',
    assessment                  TEXT NOT NULL DEFAULT '',
    plan                        TEXT NOT NULL DEFAULT '',
    status                      TEXT NOT NULL DEFAULT 'in_progress' CHECK (status IN ('in_progress', 'completed')),
    next_review_date            DATE,
    follow_up_appointment_id    BIGINT REFERENCES appointments(id),
    review_by_practitioner_id   BIGINT REFERENCES practitioners(id),
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_encounters_patient ON encounters (patient_id, created_at);
CREATE INDEX idx_encounters_appointment ON encounters (appointment_id);
CREATE INDEX idx_encounters_company ON encounters (company_id);

CREATE TABLE vital_signs (
    id             BIGSERIAL PRIMARY KEY,
    encounter_id   BIGINT NOT NULL UNIQUE REFERENCES encounters(id) ON DELETE CASCADE,
    height_cm      NUMERIC,
    weight_kg      NUMERIC,
    bmi            NUMERIC,
    temperature_c  NUMERIC,
    pulse_bpm      INT,
    resp_rate      INT,
    bp_systolic    INT,
    bp_diastolic   INT,
    spo2           INT,
    recorded_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Global (not per-company) — ICD-10 is a fixed international standard, not
-- a clinic-specific catalog, unlike drugs/lab_test_templates/procedure_templates.
-- Seeded with a small, practically useful OPD starter set below, not the
-- full ~70,000-code catalog; diagnoses always allow a free-text description
-- alongside (or instead of) a coded lookup.
CREATE TABLE icd10_codes (
    code         TEXT PRIMARY KEY,
    description  TEXT NOT NULL
);

INSERT INTO icd10_codes (code, description) VALUES
    ('J06.9', 'Acute upper respiratory infection, unspecified'),
    ('J00', 'Acute nasopharyngitis (common cold)'),
    ('J02.9', 'Acute pharyngitis, unspecified'),
    ('J03.90', 'Acute tonsillitis, unspecified'),
    ('J20.9', 'Acute bronchitis, unspecified'),
    ('J45.909', 'Unspecified asthma, uncomplicated'),
    ('I10', 'Essential (primary) hypertension'),
    ('E11.9', 'Type 2 diabetes mellitus without complications'),
    ('E78.5', 'Hyperlipidemia, unspecified'),
    ('K21.9', 'Gastro-esophageal reflux disease without esophagitis'),
    ('K29.70', 'Gastritis, unspecified, without bleeding'),
    ('K59.00', 'Constipation, unspecified'),
    ('R10.9', 'Unspecified abdominal pain'),
    ('R51', 'Headache'),
    ('R50.9', 'Fever, unspecified'),
    ('R05.9', 'Cough, unspecified'),
    ('R11.10', 'Vomiting, unspecified'),
    ('M54.5', 'Low back pain'),
    ('M25.50', 'Pain in unspecified joint'),
    ('M79.10', 'Myalgia, unspecified site'),
    ('L23.9', 'Allergic contact dermatitis, unspecified cause'),
    ('L30.9', 'Dermatitis, unspecified'),
    ('B34.9', 'Viral infection, unspecified'),
    ('A09', 'Infectious gastroenteritis and colitis, unspecified'),
    ('N39.0', 'Urinary tract infection, site not specified'),
    ('H10.9', 'Unspecified conjunctivitis'),
    ('H66.90', 'Otitis media, unspecified, unspecified ear'),
    ('F41.9', 'Anxiety disorder, unspecified'),
    ('F32.9', 'Major depressive disorder, single episode, unspecified'),
    ('Z00.00', 'Encounter for general adult medical examination without abnormal findings');

CREATE TABLE diagnoses (
    id              BIGSERIAL PRIMARY KEY,
    encounter_id    BIGINT NOT NULL REFERENCES encounters(id) ON DELETE CASCADE,
    icd10_code      TEXT REFERENCES icd10_codes(code),
    description     TEXT NOT NULL,
    diagnosis_type  TEXT NOT NULL DEFAULT 'primary' CHECK (diagnosis_type IN ('primary', 'secondary')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_diagnoses_encounter ON diagnoses (encounter_id);

-- One prescription header per encounter (auto-created alongside it),
-- mirroring the Quick Prescription header; prescription_items are the line
-- items (Quick Prescription Medication).
CREATE TABLE prescriptions (
    id               BIGSERIAL PRIMARY KEY,
    encounter_id     BIGINT NOT NULL UNIQUE REFERENCES encounters(id) ON DELETE CASCADE,
    patient_id       BIGINT NOT NULL REFERENCES patients(id),
    practitioner_id  BIGINT NOT NULL REFERENCES practitioners(id),
    prescribed_date  DATE NOT NULL DEFAULT CURRENT_DATE,
    notes            TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE prescription_items (
    id               BIGSERIAL PRIMARY KEY,
    prescription_id  BIGINT NOT NULL REFERENCES prescriptions(id) ON DELETE CASCADE,
    drug_id          BIGINT REFERENCES drugs(id),
    drug_name        TEXT NOT NULL,
    dosage_id        BIGINT REFERENCES lookup_values(id),
    dosage_text      TEXT NOT NULL DEFAULT '',
    duration_id      BIGINT REFERENCES lookup_values(id),
    duration_text    TEXT NOT NULL DEFAULT '',
    -- Not part of the original Quick Prescription Medication shape (which
    -- only carries dosage/duration instructions, not a stock quantity) —
    -- added because M7's promised stock auto-decrement on dispense is
    -- meaningless without an actual quantity to decrement by. Optional:
    -- a linked drug with no quantity given simply isn't stock-decremented.
    quantity         NUMERIC,
    remark           TEXT NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_prescription_items_prescription ON prescription_items (prescription_id);

CREATE TABLE lab_orders (
    id                     BIGSERIAL PRIMARY KEY,
    encounter_id           BIGINT NOT NULL REFERENCES encounters(id) ON DELETE CASCADE,
    patient_id             BIGINT NOT NULL REFERENCES patients(id),
    lab_test_template_id   BIGINT REFERENCES lab_test_templates(id),
    test_name              TEXT NOT NULL,
    comment                TEXT NOT NULL DEFAULT '',
    status                 TEXT NOT NULL DEFAULT 'ordered' CHECK (status IN ('ordered', 'collected', 'resulted')),
    result_value           TEXT NOT NULL DEFAULT '',
    result_unit            TEXT NOT NULL DEFAULT '',
    reference_range        TEXT NOT NULL DEFAULT '',
    result_notes           TEXT NOT NULL DEFAULT '',
    ordered_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    resulted_at            TIMESTAMPTZ
);

CREATE INDEX idx_lab_orders_encounter ON lab_orders (encounter_id);
CREATE INDEX idx_lab_orders_patient ON lab_orders (patient_id, ordered_at);

CREATE TABLE procedure_orders (
    id                      BIGSERIAL PRIMARY KEY,
    encounter_id            BIGINT NOT NULL REFERENCES encounters(id) ON DELETE CASCADE,
    patient_id              BIGINT NOT NULL REFERENCES patients(id),
    procedure_template_id   BIGINT REFERENCES procedure_templates(id),
    procedure_name          TEXT NOT NULL,
    comments                TEXT NOT NULL DEFAULT '',
    status                  TEXT NOT NULL DEFAULT 'ordered' CHECK (status IN ('ordered', 'completed')),
    ordered_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_procedure_orders_encounter ON procedure_orders (encounter_id);
CREATE INDEX idx_procedure_orders_patient ON procedure_orders (patient_id, ordered_at);

-- +goose Down
DROP TABLE procedure_orders;
DROP TABLE lab_orders;
DROP TABLE prescription_items;
DROP TABLE prescriptions;
DROP TABLE diagnoses;
DROP TABLE icd10_codes;
DROP TABLE vital_signs;
DROP TABLE encounters;
