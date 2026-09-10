-- +goose Up
-- Rounding out a login's personal info to match what the patient record
-- already collects — address and an emergency contact, alongside the
-- full_name/phone added in 0017.
ALTER TABLE users ADD COLUMN address TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN emergency_contact_name TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN emergency_contact_phone TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE users DROP COLUMN address;
ALTER TABLE users DROP COLUMN emergency_contact_name;
ALTER TABLE users DROP COLUMN emergency_contact_phone;
