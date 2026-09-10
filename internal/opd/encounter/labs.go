package encounter

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type LabOrder struct {
	ID                int64
	EncounterID       int64
	LabTestTemplateID *int64
	TestName          string
	Comment           string
	Status            string
	ResultValue       string
	ResultUnit        string
	ReferenceRange    string
	ResultNotes       string
	OrderedAt         time.Time
	ResultedAt        *time.Time
}

type LabOrderInput struct {
	LabTestTemplateID *int64
	TestName          string
	Comment           string
}

type LabResultInput struct {
	ResultValue    string
	ResultUnit     string
	ReferenceRange string
	ResultNotes    string
}

const labOrderColumns = `id, encounter_id, lab_test_template_id, test_name, comment, status,
	result_value, result_unit, reference_range, result_notes, ordered_at, resulted_at`

func (s *Store) ListLabOrders(ctx context.Context, encounterID int64) ([]LabOrder, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+labOrderColumns+` FROM lab_orders WHERE encounter_id = $1 ORDER BY ordered_at`, encounterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LabOrder
	for rows.Next() {
		lo, err := scanLabOrder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *lo)
	}
	return out, rows.Err()
}

func scanLabOrder(row pgx.Row) (*LabOrder, error) {
	var lo LabOrder
	err := row.Scan(&lo.ID, &lo.EncounterID, &lo.LabTestTemplateID, &lo.TestName, &lo.Comment, &lo.Status,
		&lo.ResultValue, &lo.ResultUnit, &lo.ReferenceRange, &lo.ResultNotes, &lo.OrderedAt, &lo.ResultedAt)
	if err != nil {
		return nil, err
	}
	return &lo, nil
}

func (s *Store) AddLabOrder(ctx context.Context, encounterID, patientID int64, in LabOrderInput) error {
	tag, err := s.Pool.Exec(ctx, `
		INSERT INTO lab_orders (encounter_id, patient_id, lab_test_template_id, test_name, comment)
		SELECT $1, $2, $3, $4, $5
		WHERE EXISTS (SELECT 1 FROM encounters WHERE id = $1 AND status = 'in_progress')`,
		encounterID, patientID, in.LabTestTemplateID, in.TestName, in.Comment)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCompleted
	}
	return nil
}

// RecordLabResult is not status-scoped — results often come back after
// the visit ends.
func (s *Store) RecordLabResult(ctx context.Context, id, encounterID int64, in LabResultInput) error {
	tag, err := s.Pool.Exec(ctx, `
		UPDATE lab_orders lo SET status = 'resulted', result_value = $3, result_unit = $4,
			reference_range = $5, result_notes = $6, resulted_at = now()
		WHERE lo.id = $1 AND lo.encounter_id = $2`,
		id, encounterID, in.ResultValue, in.ResultUnit, in.ReferenceRange, in.ResultNotes)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCompleted
	}
	return nil
}

func (s *Store) DeleteLabOrder(ctx context.Context, id, encounterID int64) error {
	tag, err := s.Pool.Exec(ctx, `
		DELETE FROM lab_orders lo USING encounters e
		WHERE lo.id = $1 AND lo.encounter_id = $2 AND e.id = lo.encounter_id
		  AND e.status = 'in_progress'`,
		id, encounterID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCompleted
	}
	return nil
}
