-- +goose Up
-- Online bookings (source='online') have no staff acting user — the visitor
-- isn't logged in — so created_by must allow NULL for that path. Staff
-- bookings continue to always set it.
ALTER TABLE appointments ALTER COLUMN created_by DROP NOT NULL;

-- +goose Down
ALTER TABLE appointments ALTER COLUMN created_by SET NOT NULL;
