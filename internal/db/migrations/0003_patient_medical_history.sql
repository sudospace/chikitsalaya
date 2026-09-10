-- +goose Up
ALTER TABLE patients ADD COLUMN medical_history TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE patients DROP COLUMN medical_history;
