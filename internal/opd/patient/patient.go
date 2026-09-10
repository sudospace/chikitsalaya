// Package patient implements patient records: CRUD and search.
package patient

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Patient struct {
	ID                    int64
	MRN                   string
	FirstName             string
	LastName              string
	Age                   *int
	Sex                   string
	Phone                 string
	Email                 string
	Address               string
	EmergencyContactName  string
	EmergencyContactPhone string
	BloodGroup            string
	Allergies             string
	MedicalHistory        string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// Input holds the editable patient fields coming from a form submission.
type Input struct {
	FirstName             string
	LastName              string
	Age                   *int
	Sex                   string
	Phone                 string
	Email                 string
	Address               string
	EmergencyContactName  string
	EmergencyContactPhone string
	BloodGroup            string
	Allergies             string
	MedicalHistory        string
}

// SearchWidgetConfig configures the shared patient search-or-create widget
// template (see patient_search_widget.html) for one page's context.
type SearchWidgetConfig struct {
	Mode             string // "embedded" (fields live in the page's own <form>) or "standalone" (widget wraps its own <form>)
	SearchEndpoint   string
	NavigateTemplate string // e.g. "/appointments/new?patient_id={id}" — used when a pick should navigate instead of filling a hidden field
	ActionURL        string // only used when Mode == "standalone"; the new-patient panel's <form action=...>
}

type Store struct {
	Pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool}
}

const selectColumns = `id, mrn, first_name, last_name, age, sex, phone, email, address,
	emergency_contact_name, emergency_contact_phone, blood_group, allergies, medical_history,
	created_at, updated_at`

// List returns patients matching q against name, phone, or MRN, or the most
// recent patients (capped) when q is empty.
func (s *Store) List(ctx context.Context, q string) ([]Patient, error) {
	var rows pgx.Rows
	var err error
	if q == "" {
		rows, err = s.Pool.Query(ctx,
			`SELECT `+selectColumns+` FROM patients ORDER BY last_name, first_name LIMIT 200`)
	} else {
		pattern := "%" + q + "%"
		rows, err = s.Pool.Query(ctx,
			`SELECT `+selectColumns+` FROM patients
			 WHERE first_name ILIKE $1 OR last_name ILIKE $1 OR phone ILIKE $1 OR mrn ILIKE $1
			 ORDER BY last_name, first_name LIMIT 50`, pattern)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var patients []Patient
	for rows.Next() {
		p, err := scanPatient(rows)
		if err != nil {
			return nil, err
		}
		patients = append(patients, *p)
	}
	return patients, rows.Err()
}

func (s *Store) Get(ctx context.Context, id int64) (*Patient, error) {
	row := s.Pool.QueryRow(ctx, `SELECT `+selectColumns+` FROM patients WHERE id = $1`, id)
	return scanPatient(row)
}

// FindByPhone links a returning online-booking visitor to their existing record
// instead of creating a duplicate — phone is a weak key but it's what OTP verification collects.
func (s *Store) FindByPhone(ctx context.Context, phone string) (*Patient, error) {
	row := s.Pool.QueryRow(ctx,
		`SELECT `+selectColumns+` FROM patients WHERE phone = $1 ORDER BY updated_at DESC LIMIT 1`, phone)
	return scanPatient(row)
}

func scanPatient(row pgx.Row) (*Patient, error) {
	var p Patient
	err := row.Scan(&p.ID, &p.MRN, &p.FirstName, &p.LastName, &p.Age, &p.Sex, &p.Phone, &p.Email,
		&p.Address, &p.EmergencyContactName, &p.EmergencyContactPhone, &p.BloodGroup, &p.Allergies,
		&p.MedicalHistory, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Create assigns the MRN from the row's own id (e.g. "P000042") instead of
// a separate counter table.
func (s *Store) Create(ctx context.Context, in Input) (int64, error) {
	var id int64
	if err := s.Pool.QueryRow(ctx,
		`SELECT nextval(pg_get_serial_sequence('patients', 'id'))`).Scan(&id); err != nil {
		return 0, fmt.Errorf("reserve id: %w", err)
	}
	mrn := fmt.Sprintf("P%06d", id)

	_, err := s.Pool.Exec(ctx, `
		INSERT INTO patients (id, mrn, first_name, last_name, age, sex, phone, email, address,
			emergency_contact_name, emergency_contact_phone, blood_group, allergies, medical_history)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		id, mrn, in.FirstName, in.LastName, in.Age, in.Sex, in.Phone, in.Email, in.Address,
		in.EmergencyContactName, in.EmergencyContactPhone, in.BloodGroup, in.Allergies, in.MedicalHistory)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) Update(ctx context.Context, id int64, in Input) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE patients SET
			first_name = $2, last_name = $3, age = $4, sex = $5, phone = $6, email = $7,
			address = $8, emergency_contact_name = $9, emergency_contact_phone = $10,
			blood_group = $11, allergies = $12, medical_history = $13, updated_at = now()
		WHERE id = $1`,
		id, in.FirstName, in.LastName, in.Age, in.Sex, in.Phone, in.Email, in.Address,
		in.EmergencyContactName, in.EmergencyContactPhone, in.BloodGroup, in.Allergies, in.MedicalHistory)
	return err
}
