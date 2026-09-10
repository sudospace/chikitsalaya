// Package auth provides session-based authentication and role-guard
// middleware shared by every clinical module.
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// Role hierarchy: admin (full rights over the clinic) -> doctor /
// receptionist (day-to-day staff).
const (
	RoleAdmin        = "admin"
	RoleDoctor       = "doctor"
	RoleReceptionist = "receptionist"
)

var ErrInvalidCredentials = errors.New("invalid email or password")

type User struct {
	ID                    int64
	Email                 string
	PasswordHash          string
	Role                  string
	PractitionerID        *int64
	FullName              string
	Phone                 string
	Address               string
	EmergencyContactName  string
	EmergencyContactPhone string
	IsActive              bool
	OnboardedAt           *time.Time
	TOTPSecret            *string    // nil until 2FA enrollment starts
	TOTPEnabledAt         *time.Time // nil until enrollment is confirmed with a valid code
}

// TOTPConfirmed reports whether this user has completed 2FA enrollment
// (as opposed to having a secret generated but never confirmed).
func (u *User) TOTPConfirmed() bool {
	return u.TOTPEnabledAt != nil
}

// Store provides user lookups and creation backed by Postgres.
type Store struct {
	Pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool}
}

const userColumns = `id, email, password_hash, role, practitioner_id, full_name, phone,
	address, emergency_contact_name, emergency_contact_phone, is_active, onboarded_at,
	totp_secret, totp_enabled_at`

func (s *Store) GetByEmail(ctx context.Context, email string) (*User, error) {
	row := s.Pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE email = $1`, email)
	return scanUser(row)
}

func (s *Store) GetByID(ctx context.Context, id int64) (*User, error) {
	row := s.Pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id)
	return scanUser(row)
}

// ListAll returns every user, for the admin's "Users" screen.
func (s *Store) ListAll(ctx context.Context) ([]User, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+userColumns+` FROM users ORDER BY role, email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

func scanUser(row pgx.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.PractitionerID,
		&u.FullName, &u.Phone, &u.Address, &u.EmergencyContactName, &u.EmergencyContactPhone,
		&u.IsActive, &u.OnboardedAt, &u.TOTPSecret, &u.TOTPEnabledAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// Authenticate looks up the user by email and verifies the password. It
// returns ErrInvalidCredentials for both "no such user" and "wrong
// password" so callers can't distinguish the two from the error alone.
func (s *Store) Authenticate(ctx context.Context, email, password string) (*User, error) {
	u, err := s.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if !u.IsActive {
		return nil, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}
	return u, nil
}

// UpsertAdmin creates the clinic's admin user if the email doesn't exist
// yet, or resets its password/role if it does. Used by the --seed-admin
// CLI bootstrap. Returns the user's id so the caller can grant it
// permissions right after.
func (s *Store) UpsertAdmin(ctx context.Context, email, password string) (int64, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, fmt.Errorf("hash password: %w", err)
	}
	var id int64
	err = s.Pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, role, is_active)
		VALUES ($1, $2, $3, TRUE)
		ON CONFLICT (email) DO UPDATE
			SET password_hash = EXCLUDED.password_hash,
			    role = EXCLUDED.role,
			    is_active = TRUE
		RETURNING id`,
		email, string(hash), RoleAdmin).Scan(&id)
	return id, err
}

// CreateInput describes a new staff login created through the app (an
// admin creating another admin, doctor, or receptionist).
type CreateInput struct {
	Email                 string
	Password              string
	Role                  string
	PractitionerID        *int64
	FullName              string
	Phone                 string
	Address               string
	EmergencyContactName  string
	EmergencyContactPhone string
}

func (s *Store) Create(ctx context.Context, in CreateInput) (int64, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return 0, fmt.Errorf("hash password: %w", err)
	}
	var id int64
	err = s.Pool.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, role, practitioner_id, full_name, phone,
			address, emergency_contact_name, emergency_contact_phone, is_active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,TRUE) RETURNING id`,
		in.Email, string(hash), in.Role, in.PractitionerID, in.FullName, in.Phone,
		in.Address, in.EmergencyContactName, in.EmergencyContactPhone).Scan(&id)
	return id, err
}

// MarkOnboarded records that an admin has finished (or skipped) their
// one-time first-login wizard.
func (s *Store) MarkOnboarded(ctx context.Context, id int64) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET onboarded_at = now() WHERE id = $1`, id)
	return err
}

// SetActive flips a login on/off without deleting it.
func (s *Store) SetActive(ctx context.Context, id int64, active bool) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE users SET is_active = $2 WHERE id = $1`, id, active)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// SetPassword replaces a user's own password hash — the self-service
// "change password" action on their account page. Callers must verify
// the current password themselves first (see Authenticate); this just
// writes the new hash.
func (s *Store) SetPassword(ctx context.Context, id int64, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	_, err = s.Pool.Exec(ctx, `UPDATE users SET password_hash = $2 WHERE id = $1`, id, string(hash))
	return err
}

// LinkPractitioner sets (or, with nil, clears) which practitioner record
// this account can act as via SetActingAsDoctor. Self-service path for an
// admin who is also a doctor but wasn't created through the doctor-signup
// flow.
func (s *Store) LinkPractitioner(ctx context.Context, userID int64, practitionerID *int64) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET practitioner_id = $2 WHERE id = $1`, userID, practitionerID)
	return err
}

// PersonalInput holds the editable "who is this person" fields, shared by
// self-service and admin-edited updates.
type PersonalInput struct {
	FullName              string
	Phone                 string
	Address               string
	EmergencyContactName  string
	EmergencyContactPhone string
}

// UpdatePersonal writes a user's personal-info fields. No permission check
// here — the caller must have already verified id is theirs to edit.
func (s *Store) UpdatePersonal(ctx context.Context, id int64, in PersonalInput) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE users SET full_name = $2, phone = $3, address = $4,
			emergency_contact_name = $5, emergency_contact_phone = $6
		WHERE id = $1`,
		id, in.FullName, in.Phone, in.Address, in.EmergencyContactName, in.EmergencyContactPhone)
	return err
}

// SetTOTPSecret records a freshly generated (not yet confirmed) TOTP
// secret, starting/restarting 2FA enrollment for this user.
func (s *Store) SetTOTPSecret(ctx context.Context, id int64, secret string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE users SET totp_secret = $2, totp_enabled_at = NULL WHERE id = $1`, id, secret)
	return err
}

// EnableTOTP marks 2FA enrollment as confirmed after the user has proven
// they can generate a valid code.
func (s *Store) EnableTOTP(ctx context.Context, id int64) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET totp_enabled_at = now() WHERE id = $1`, id)
	return err
}
