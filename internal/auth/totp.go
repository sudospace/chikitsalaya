package auth

import (
	"encoding/base64"
	"fmt"
	"net/url"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/skip2/go-qrcode"
)

const totpIssuer = "Chikitsalaya"

// GenerateTOTPSecret creates a random TOTP secret for an enrollment
// attempt; callers persist it via Store.SetTOTPSecret.
func GenerateTOTPSecret(accountEmail string) (*otp.Key, error) {
	return totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: accountEmail,
	})
}

// otpauthURL rebuilds the enrollment URL for an already-stored base32
// secret directly, rather than via totp.Generate's Secret option, which
// takes raw bytes and would silently re-encode to a different secret.
func otpauthURL(accountEmail, base32Secret string) string {
	v := url.Values{}
	v.Set("secret", base32Secret)
	v.Set("issuer", totpIssuer)
	v.Set("algorithm", "SHA1")
	v.Set("digits", "6")
	v.Set("period", "30")
	u := url.URL{
		Scheme:   "otpauth",
		Host:     "totp",
		Path:     "/" + totpIssuer + ":" + accountEmail,
		RawQuery: v.Encode(),
	}
	return u.String()
}

// TOTPQRCodeDataURI renders the enrollment otpauth:// URI as a PNG QR code,
// base64-embedded so the setup page needs no extra image route.
func TOTPQRCodeDataURI(otpauthURL string) (string, error) {
	png, err := qrcode.Encode(otpauthURL, qrcode.Medium, 256)
	if err != nil {
		return "", fmt.Errorf("render QR code: %w", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

// ValidateTOTPCode checks a 6-digit code against a stored secret.
func ValidateTOTPCode(code, secret string) bool {
	return totp.Validate(code, secret)
}
