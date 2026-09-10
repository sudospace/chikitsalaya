// Package encounter implements the consultation workspace: the encounter
// record itself plus everything captured during one visit — vitals,
// diagnoses, a prescription, and lab/procedure orders.
package encounter

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrCompleted = errors.New("this encounter is completed and can no longer be edited")

type Encounter struct {
	ID                       int64
	AppointmentID            *int64
	PatientID                int64
	PatientName              string
	PractitionerID           int64
	PractitionerName         string
	ChiefComplaint           string
	HistoryOfPresentIllness  string
	ExaminationNotes         string
	Assessment               string
	Plan                     string
	Status                   string
	NextReviewDate           *time.Time
	FollowUpAppointmentID    *int64
	ReviewByPractitionerID   *int64
	ReviewByPractitionerName string
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// CoreInput holds the free-text consultation fields plus the
// review/follow-up bits, all editable together on the workspace page.
type CoreInput struct {
	ChiefComplaint          string
	HistoryOfPresentIllness string
	ExaminationNotes        string
	Assessment              string
	Plan                    string
	NextReviewDate          *time.Time
	ReviewByPractitionerID  *int64
}

type Store struct {
	Pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool}
}

const selectBase = `SELECT e.id, e.appointment_id, e.patient_id, p.first_name || ' ' || p.last_name,
	e.practitioner_id, pr.full_name, e.chief_complaint, e.history_of_present_illness,
	e.examination_notes, e.assessment, e.plan, e.status, e.next_review_date, e.follow_up_appointment_id,
	e.review_by_practitioner_id, COALESCE(rp.full_name, ''), e.created_at, e.updated_at
	FROM encounters e
	JOIN patients p ON p.id = e.patient_id
	JOIN practitioners pr ON pr.id = e.practitioner_id
	LEFT JOIN practitioners rp ON rp.id = e.review_by_practitioner_id`

func (s *Store) Get(ctx context.Context, id int64) (*Encounter, error) {
	row := s.Pool.QueryRow(ctx, selectBase+` WHERE e.id = $1`, id)
	return scan(row)
}

// ListByPatient returns every encounter for a patient, most recent first.
func (s *Store) ListByPatient(ctx context.Context, patientID int64) ([]Encounter, error) {
	rows, err := s.Pool.Query(ctx, selectBase+` WHERE e.patient_id = $1 ORDER BY e.created_at DESC`, patientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Encounter
	for rows.Next() {
		e, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func scan(row pgx.Row) (*Encounter, error) {
	var e Encounter
	err := row.Scan(&e.ID, &e.AppointmentID, &e.PatientID, &e.PatientName, &e.PractitionerID,
		&e.PractitionerName, &e.ChiefComplaint, &e.HistoryOfPresentIllness,
		&e.ExaminationNotes, &e.Assessment, &e.Plan, &e.Status, &e.NextReviewDate,
		&e.FollowUpAppointmentID, &e.ReviewByPractitionerID, &e.ReviewByPractitionerName,
		&e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// StartFromAppointment returns the existing encounter for this appointment,
// or creates one (plus its 1:1 prescription header) — so re-clicking
// "Start Consultation" resumes rather than duplicates.
func (s *Store) StartFromAppointment(ctx context.Context, appointmentID, patientID, practitionerID int64) (int64, error) {
	var existingID int64
	err := s.Pool.QueryRow(ctx, `SELECT id FROM encounters WHERE appointment_id = $1`, appointmentID).Scan(&existingID)
	if err == nil {
		return existingID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO encounters (appointment_id, patient_id, practitioner_id)
		VALUES ($1,$2,$3) RETURNING id`,
		appointmentID, patientID, practitionerID).Scan(&id)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO prescriptions (encounter_id, patient_id, practitioner_id)
		VALUES ($1,$2,$3)`, id, patientID, practitionerID); err != nil {
		return 0, err
	}

	return id, tx.Commit(ctx)
}

func (s *Store) UpdateCore(ctx context.Context, id int64, in CoreInput) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE encounters SET
			chief_complaint = $2, history_of_present_illness = $3, examination_notes = $4,
			assessment = $5, plan = $6, next_review_date = $7, review_by_practitioner_id = $8,
			updated_at = now()
		WHERE id = $1 AND status = 'in_progress'`,
		id, in.ChiefComplaint, in.HistoryOfPresentIllness, in.ExaminationNotes,
		in.Assessment, in.Plan, in.NextReviewDate, in.ReviewByPractitionerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCompleted
	}
	return nil
}

// Complete locks the encounter against further edits. Stock dispensing is a
// separate call (Store.DispenseStock, prescription.go) made alongside this.
func (s *Store) Complete(ctx context.Context, id int64) error {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE encounters SET status = 'completed', updated_at = now()
		 WHERE id = $1 AND status = 'in_progress'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCompleted
	}
	return nil
}

// ErrNotCompleted is returned by Reopen when the encounter isn't
// currently completed — nothing to reopen.
var ErrNotCompleted = errors.New("this encounter isn't completed")

// Reopen puts a completed encounter back to in_progress so it can be
// corrected; re-completing afterward is safe since DispenseStock skips
// items it already dispensed.
func (s *Store) Reopen(ctx context.Context, id int64) error {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE encounters SET status = 'in_progress', updated_at = now()
		 WHERE id = $1 AND status = 'completed'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotCompleted
	}
	return nil
}
