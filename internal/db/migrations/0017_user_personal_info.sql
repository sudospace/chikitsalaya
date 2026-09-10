-- +goose Up
-- Every account gets a name/phone of its own now, not just an email — the
-- "Personal" section of the self-service account page needs somewhere to
-- read and write, and it applies to every role (master/company_admin/
-- receptionist), not just doctors (who already have this on their
-- practitioner record).
ALTER TABLE users ADD COLUMN full_name TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN phone TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE users DROP COLUMN full_name;
ALTER TABLE users DROP COLUMN phone;
