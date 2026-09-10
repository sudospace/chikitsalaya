-- +goose Up
-- Mandatory TOTP two-factor auth: a NULL totp_secret means "not enrolled
-- yet" (forced through /2fa/setup on next login); a non-NULL secret with
-- totp_enabled_at still NULL means "enrolled but never confirmed" (also
-- routed back to /2fa/setup rather than /2fa/verify, since we can't be sure
-- the user actually saved the QR code).
ALTER TABLE users ADD COLUMN totp_secret TEXT;
ALTER TABLE users ADD COLUMN totp_enabled_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE users DROP COLUMN totp_enabled_at;
ALTER TABLE users DROP COLUMN totp_secret;
