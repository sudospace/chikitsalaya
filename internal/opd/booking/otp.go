package booking

import (
	"context"
	"crypto/rand"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const otpTTL = 5 * time.Minute

type OTPStore struct {
	Pool *pgxpool.Pool
}

func NewOTPStore(pool *pgxpool.Pool) *OTPStore {
	return &OTPStore{Pool: pool}
}

// CreateOTP generates and stores a fresh 6-digit code for mobile, valid
// for otpTTL. Every request gets its own row (no reuse/overwrite of a
// prior one) — VerifyOTP always checks against the latest.
func (s *OTPStore) CreateOTP(ctx context.Context, mobile string) (string, error) {
	code, err := randomDigits(6)
	if err != nil {
		return "", err
	}
	_, err = s.Pool.Exec(ctx,
		`INSERT INTO appointment_otps (mobile, otp, expires_at) VALUES ($1, $2, $3)`,
		mobile, code, time.Now().Add(otpTTL))
	if err != nil {
		return "", err
	}
	return code, nil
}

// VerifyOTP checks code against the most recent unverified, unexpired OTP
// issued for mobile. On a match it marks that row verified (so it can't be
// replayed) and returns true.
func (s *OTPStore) VerifyOTP(ctx context.Context, mobile, code string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE appointment_otps SET verified_at = now()
		WHERE id = (
			SELECT id FROM appointment_otps
			WHERE mobile = $1 AND otp = $2 AND verified_at IS NULL AND expires_at > now()
			ORDER BY created_at DESC LIMIT 1
		)`, mobile, code)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func randomDigits(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i, v := range b {
		out[i] = '0' + v%10
	}
	return string(out), nil
}
