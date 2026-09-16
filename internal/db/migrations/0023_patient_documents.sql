-- +goose Up
-- Multiple uploadable report files per patient (lab/scan PDFs, images,
-- etc.), independent of the encounter/visit timeline. storage_backend is
-- recorded per row (not read from "whatever's currently active") so
-- switching the clinic's active backend later doesn't strand old files.
CREATE TABLE patient_documents (
    id                 BIGSERIAL PRIMARY KEY,
    patient_id         BIGINT NOT NULL REFERENCES patients(id) ON DELETE CASCADE,
    original_filename  TEXT NOT NULL,
    storage_backend    TEXT NOT NULL CHECK (storage_backend IN ('local', 'gdrive', 's3')),
    storage_key        TEXT NOT NULL,
    content_type       TEXT NOT NULL DEFAULT 'application/octet-stream',
    size_bytes         BIGINT NOT NULL DEFAULT 0,
    description        TEXT NOT NULL DEFAULT '',
    uploaded_by        BIGINT REFERENCES users(id),
    uploaded_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_patient_documents_patient ON patient_documents (patient_id, uploaded_at);

-- +goose Down
DROP TABLE patient_documents;
