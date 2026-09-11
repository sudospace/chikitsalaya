-- +goose Up
-- Financial-year start month for the reports date-range presets (e.g. 4 =
-- April-March, common for Indian statutory reporting). Configurable rather
-- than hardcoded since nothing else in this app assumes a particular
-- jurisdiction's fiscal calendar.
ALTER TABLE clinic_settings ADD COLUMN fy_start_month SMALLINT NOT NULL DEFAULT 4;

-- +goose Down
ALTER TABLE clinic_settings DROP COLUMN fy_start_month;
