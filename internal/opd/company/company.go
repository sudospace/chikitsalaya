// Package company implements the single clinic's settings and its
// physical rooms ("service units") — the masters other OPD records
// (practitioners, appointments, encounters) attach to.
package company

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Company struct {
	ID                   int64
	Name                 string
	Address              string
	Phone                string
	Email                string
	RegistrationNo       string
	LogoPath             string
	LetterheadMode       string // "html" or "image"
	LetterheadHTML       string
	LetterheadImagePath  string
	FooterText           string
	FirstConsultationFee *float64
	FollowUpFee          *float64
	GSTIN                string
	GSTRegistered        bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// CompanyInput holds the editable form fields. LogoPath/LetterheadImagePath
// are absent — each is set only via its own Store.Set*Path (see logo.go).
type CompanyInput struct {
	Name                 string
	Address              string
	Phone                string
	Email                string
	RegistrationNo       string
	LetterheadMode       string
	LetterheadHTML       string
	FooterText           string
	FirstConsultationFee *float64
	FollowUpFee          *float64
	GSTIN                string
	GSTRegistered        bool
}

type ServiceUnit struct {
	ID        int64
	Name      string
	UnitType  string
	IsActive  bool
	CreatedAt time.Time
}

type ServiceUnitInput struct {
	Name     string
	UnitType string
	IsActive bool
}

type Store struct {
	Pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool}
}

const companyColumns = `id, name, address, phone, email, registration_no,
	logo_path, letterhead_mode, letterhead_html, letterhead_image_path, footer_text,
	first_consultation_fee, follow_up_fee, gstin, gst_registered, created_at, updated_at`

// defaultLookupValues seeds the clinic with a starting dosage/duration
// picklist, editable afterward via /admin/lookup-values.
var defaultLookupValues = []struct {
	Category  string
	Value     string
	SortOrder int
}{
	{"dosage", "1-0-0", 1},
	{"dosage", "0-1-0", 2},
	{"dosage", "0-0-1", 3},
	{"dosage", "1-0-1", 4},
	{"dosage", "1-1-1", 5},
	{"dosage", "1-1-1-1", 6},
	{"dosage", "Once Daily", 7},
	{"dosage", "Once Bedtime", 8},
	{"dosage", "BID", 9},
	{"dosage", "TID", 10},
	{"dosage", "QID", 11},
	{"dosage", "5 times a day", 12},
	{"duration", "1 Hour", 1},
	{"duration", "2 Hour", 2},
	{"duration", "3 Hour", 3},
	{"duration", "4 Hour", 4},
	{"duration", "5 Hour", 5},
	{"duration", "6 Hour", 6},
	{"duration", "7 Hour", 7},
	{"duration", "8 Hour", 8},
	{"duration", "9 Hour", 9},
	{"duration", "10 Hour", 10},
	{"duration", "11 Hour", 11},
	{"duration", "12 Hour", 12},
	{"duration", "1 Day", 13},
	{"duration", "2 Day", 14},
	{"duration", "3 Day", 15},
	{"duration", "4 Day", 16},
	{"duration", "5 Day", 17},
	{"duration", "6 Day", 18},
	{"duration", "1 Week", 19},
	{"duration", "2 Week", 20},
	{"duration", "3 Week", 21},
	{"duration", "4 Week", 22},
	{"duration", "5 Week", 23},
	{"duration", "1 Month", 24},
	{"duration", "2 Month", 25},
	{"duration", "3 Month", 26},
}

// GetClinic returns the single clinic settings row. There is always exactly
// one, created by EnsureClinic during bootstrap.
func (s *Store) GetClinic(ctx context.Context) (*Company, error) {
	row := s.Pool.QueryRow(ctx, `SELECT `+companyColumns+` FROM clinic_settings LIMIT 1`)
	return scanCompany(row)
}

func scanCompany(row pgx.Row) (*Company, error) {
	var c Company
	err := row.Scan(&c.ID, &c.Name, &c.Address, &c.Phone, &c.Email, &c.RegistrationNo,
		&c.LogoPath, &c.LetterheadMode, &c.LetterheadHTML, &c.LetterheadImagePath, &c.FooterText,
		&c.FirstConsultationFee, &c.FollowUpFee, &c.GSTIN, &c.GSTRegistered, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// EnsureClinic creates the single clinic_settings row (with blank/placeholder
// fields, editable afterward from Clinic Settings) and seeds the default
// dosage/duration lookup values, but only the first time it's called —
// safe to call again on every CLI bootstrap without creating a duplicate.
func (s *Store) EnsureClinic(ctx context.Context) (int64, error) {
	var existing int64
	err := s.Pool.QueryRow(ctx, `SELECT id FROM clinic_settings LIMIT 1`).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if err != pgx.ErrNoRows {
		return 0, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var id int64
	if err := tx.QueryRow(ctx, `
		INSERT INTO clinic_settings (name, letterhead_mode)
		VALUES ('', 'html')
		RETURNING id`).Scan(&id); err != nil {
		return 0, err
	}

	for _, lv := range defaultLookupValues {
		if _, err := tx.Exec(ctx,
			`INSERT INTO lookup_values (category, value, sort_order) VALUES ($1,$2,$3)`,
			lv.Category, lv.Value, lv.SortOrder); err != nil {
			return 0, err
		}
	}

	return id, tx.Commit(ctx)
}

// SetLogoPath updates just the logo filename (see logo.go), independent of
// the rest of the clinic settings form.
func (s *Store) SetLogoPath(ctx context.Context, id int64, logoPath string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE clinic_settings SET logo_path = $2, updated_at = now() WHERE id = $1`, id, logoPath)
	return err
}

// SetLetterheadImagePath updates just the letterhead image filename (see
// letterhead.go), independent of the rest of the clinic settings form.
func (s *Store) SetLetterheadImagePath(ctx context.Context, id int64, path string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE clinic_settings SET letterhead_image_path = $2, updated_at = now() WHERE id = $1`, id, path)
	return err
}

// UpdateClinic saves the settings form. There's only ever one row, so no id
// is needed — every field is set unconditionally.
func (s *Store) UpdateClinic(ctx context.Context, in CompanyInput) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE clinic_settings SET
			name = $1, address = $2, phone = $3, email = $4, registration_no = $5,
			letterhead_mode = $6, letterhead_html = $7, footer_text = $8,
			first_consultation_fee = $9, follow_up_fee = $10, gstin = $11, gst_registered = $12, updated_at = now()`,
		in.Name, in.Address, in.Phone, in.Email, in.RegistrationNo,
		in.LetterheadMode, in.LetterheadHTML, in.FooterText, in.FirstConsultationFee, in.FollowUpFee,
		in.GSTIN, in.GSTRegistered)
	return err
}

const serviceUnitSelect = `SELECT su.id, su.name, su.unit_type, su.is_active, su.created_at FROM service_units su`

func (s *Store) ListServiceUnits(ctx context.Context) ([]ServiceUnit, error) {
	rows, err := s.Pool.Query(ctx, serviceUnitSelect+` ORDER BY su.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var units []ServiceUnit
	for rows.Next() {
		u, err := scanServiceUnit(rows)
		if err != nil {
			return nil, err
		}
		units = append(units, *u)
	}
	return units, rows.Err()
}

func (s *Store) GetServiceUnit(ctx context.Context, id int64) (*ServiceUnit, error) {
	row := s.Pool.QueryRow(ctx, serviceUnitSelect+` WHERE su.id = $1`, id)
	return scanServiceUnit(row)
}

func scanServiceUnit(row pgx.Row) (*ServiceUnit, error) {
	var u ServiceUnit
	err := row.Scan(&u.ID, &u.Name, &u.UnitType, &u.IsActive, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) CreateServiceUnit(ctx context.Context, in ServiceUnitInput) (int64, error) {
	var id int64
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO service_units (name, unit_type, is_active)
		VALUES ($1,$2,$3) RETURNING id`,
		in.Name, in.UnitType, in.IsActive).Scan(&id)
	return id, err
}

func (s *Store) UpdateServiceUnit(ctx context.Context, id int64, in ServiceUnitInput) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE service_units SET name = $2, unit_type = $3, is_active = $4
		WHERE id = $1`,
		id, in.Name, in.UnitType, in.IsActive)
	return err
}
