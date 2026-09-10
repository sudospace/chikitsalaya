-- +goose Up
-- Backs mobile OTP verification for the public online-booking page: a
-- visitor must prove they own the phone number before they can see or
-- edit any appointment tied to it. No FK to patients — the mobile might
-- belong to a brand new visitor who has no patient record yet.
CREATE TABLE appointment_otps (
    id          BIGSERIAL PRIMARY KEY,
    mobile      TEXT NOT NULL,
    otp         TEXT NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    verified_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_appointment_otps_mobile ON appointment_otps (mobile, created_at DESC);

-- +goose Down
DROP TABLE appointment_otps;
