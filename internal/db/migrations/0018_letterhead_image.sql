-- +goose Up
-- A clinic's letterhead is either typed as custom HTML (existing
-- letterhead_html) or, now, a single uploaded image (a scanned/designed
-- letterhead) — letterhead_mode picks which one the print view uses.
ALTER TABLE companies ADD COLUMN letterhead_mode TEXT NOT NULL DEFAULT 'html' CHECK (letterhead_mode IN ('html', 'image'));
ALTER TABLE companies ADD COLUMN letterhead_image_path TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE companies DROP COLUMN letterhead_mode;
ALTER TABLE companies DROP COLUMN letterhead_image_path;
