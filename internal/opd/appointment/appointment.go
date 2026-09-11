// Package appointment implements staff appointment scheduling: slot
// generation from a practitioner's weekly schedule, overlap-safe booking,
// and status transitions.
package appointment

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSlotConflict = errors.New("this slot is no longer available")
var ErrNotModifiable = errors.New("appointment not found or can no longer be changed")

type Appointment struct {
	ID               int64
	PatientID        int64
	PatientName      string
	PractitionerID   int64
	PractitionerName string
	ServiceUnitID    *int64
	ServiceUnitName  string
	ScheduledAt      time.Time
	DurationMinutes  int
	Status           string
	Source           string
	AppointmentType  string
	ReasonForVisit   string
	FeeAmount        *float64
	FeePaid          bool
	CreatedBy        *int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// CreatedBy is nil for an online booking (no logged-in staff user to record).
type Input struct {
	PatientID       int64
	PractitionerID  int64
	ServiceUnitID   *int64
	ScheduledAt     time.Time
	DurationMinutes int
	Status          string
	Source          string
	AppointmentType string
	ReasonForVisit  string
	FeeAmount       *float64
	CreatedBy       *int64
}

type Store struct {
	Pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool}
}

const selectBase = `SELECT a.id, a.patient_id, p.first_name || ' ' || p.last_name, a.practitioner_id, pr.full_name,
	a.service_unit_id, COALESCE(su.name, ''), a.scheduled_at, a.duration_minutes,
	a.status, a.source, a.appointment_type, a.reason_for_visit, a.fee_amount, a.fee_paid,
	a.created_by, a.created_at, a.updated_at
	FROM appointments a
	JOIN patients p ON p.id = a.patient_id
	JOIN practitioners pr ON pr.id = a.practitioner_id
	LEFT JOIN service_units su ON su.id = a.service_unit_id`

func dayBounds(date time.Time) (start, end time.Time) {
	start = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	return start, start.Add(24 * time.Hour)
}

// ListByPractitionerAndDate returns every appointment for that day,
// regardless of status, for the day-view display.
func (s *Store) ListByPractitionerAndDate(ctx context.Context, practitionerID int64, date time.Time) ([]Appointment, error) {
	dayStart, dayEnd := dayBounds(date)
	rows, err := s.Pool.Query(ctx, selectBase+`
		WHERE a.practitioner_id = $1 AND a.scheduled_at >= $2 AND a.scheduled_at < $3
		ORDER BY a.scheduled_at`, practitionerID, dayStart, dayEnd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Appointment
	for rows.Next() {
		a, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// ListToday excludes cancelled/no-show rows — nothing for the front desk
// to act on there.
func (s *Store) ListToday(ctx context.Context, limit int) ([]Appointment, error) {
	dayStart, dayEnd := dayBounds(time.Now())
	rows, err := s.Pool.Query(ctx, selectBase+`
		WHERE a.scheduled_at >= $1 AND a.scheduled_at < $2
		  AND a.status NOT IN ('cancelled', 'no_show')
		ORDER BY a.scheduled_at LIMIT $3`, dayStart, dayEnd, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Appointment
	for rows.Next() {
		a, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// BookedRanges returns the occupied windows for slot-conflict filtering:
// cancelled/no-show appointments free up their slot, so they're excluded.
func (s *Store) BookedRanges(ctx context.Context, practitionerID int64, date time.Time) ([]BookedRange, error) {
	dayStart, dayEnd := dayBounds(date)
	rows, err := s.Pool.Query(ctx, `
		SELECT scheduled_at, scheduled_at + (duration_minutes * interval '1 minute')
		FROM appointments
		WHERE practitioner_id = $1 AND scheduled_at >= $2 AND scheduled_at < $3
		  AND status NOT IN ('cancelled', 'no_show')`, practitionerID, dayStart, dayEnd)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BookedRange
	for rows.Next() {
		var b BookedRange
		if err := rows.Scan(&b.Start, &b.End); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) Get(ctx context.Context, id int64) (*Appointment, error) {
	row := s.Pool.QueryRow(ctx, selectBase+` WHERE a.id = $1`, id)
	return scan(row)
}

// ListByPatient is most-recent-first; upcoming vs. past is split in Go.
func (s *Store) ListByPatient(ctx context.Context, patientID int64) ([]Appointment, error) {
	rows, err := s.Pool.Query(ctx, selectBase+` WHERE a.patient_id = $1 ORDER BY a.scheduled_at DESC`, patientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Appointment
	for rows.Next() {
		a, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func scan(row pgx.Row) (*Appointment, error) {
	var a Appointment
	err := row.Scan(&a.ID, &a.PatientID, &a.PatientName, &a.PractitionerID, &a.PractitionerName,
		&a.ServiceUnitID, &a.ServiceUnitName, &a.ScheduledAt, &a.DurationMinutes,
		&a.Status, &a.Source, &a.AppointmentType, &a.ReasonForVisit, &a.FeeAmount, &a.FeePaid,
		&a.CreatedBy, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// Create re-checks for a conflicting appointment inside the same
// transaction as the insert — an application-level guard, not a hard
// guarantee under concurrent writes.
func (s *Store) Create(ctx context.Context, in Input) (int64, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	end := in.ScheduledAt.Add(time.Duration(in.DurationMinutes) * time.Minute)

	var conflictID int64
	err = tx.QueryRow(ctx, `
		SELECT id FROM appointments
		WHERE practitioner_id = $1 AND status NOT IN ('cancelled', 'no_show')
		  AND scheduled_at < $2 AND $3 < scheduled_at + (duration_minutes * interval '1 minute')
		LIMIT 1`, in.PractitionerID, end, in.ScheduledAt).Scan(&conflictID)
	if err == nil {
		return 0, ErrSlotConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO appointments (patient_id, practitioner_id, service_unit_id,
			scheduled_at, duration_minutes, status, source, appointment_type, reason_for_visit, fee_amount, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id`,
		in.PatientID, in.PractitionerID, in.ServiceUnitID, in.ScheduledAt, in.DurationMinutes,
		in.Status, in.Source, in.AppointmentType, in.ReasonForVisit, in.FeeAmount, in.CreatedBy).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

var validStatuses = map[string]bool{
	"scheduled": true, "checked_in": true, "completed": true, "cancelled": true, "no_show": true,
}

func (s *Store) UpdateStatus(ctx context.Context, id int64, status string) error {
	if !validStatuses[status] {
		return errors.New("invalid status: " + status)
	}
	_, err := s.Pool.Exec(ctx,
		`UPDATE appointments SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	return err
}

// Reschedule is scoped to patientID so a visitor can't move another's
// appointment by guessing an id. Status resets to 'requested' pending
// staff re-confirmation.
func (s *Store) Reschedule(ctx context.Context, id, patientID int64, scheduledAt time.Time, durationMinutes int, serviceUnitID *int64) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE appointments SET scheduled_at = $3, duration_minutes = $4, service_unit_id = $5,
			status = 'requested', updated_at = now()
		WHERE id = $1 AND patient_id = $2 AND status NOT IN ('completed', 'cancelled')`,
		id, patientID, scheduledAt, durationMinutes, serviceUnitID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotModifiable
	}
	return nil
}

// CancelOwned lets a visitor cancel only an appointment tied to their own
// (OTP-verified) patient record.
func (s *Store) CancelOwned(ctx context.Context, id, patientID int64) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE appointments SET status = 'cancelled', updated_at = now()
		WHERE id = $1 AND patient_id = $2 AND status NOT IN ('completed', 'cancelled')`,
		id, patientID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotModifiable
	}
	return nil
}
