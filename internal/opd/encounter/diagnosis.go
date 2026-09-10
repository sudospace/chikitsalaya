package encounter

import (
	"context"
	"time"
)

type ICD10Code struct {
	Code        string
	Description string
}

type Diagnosis struct {
	ID            int64
	EncounterID   int64
	ICD10Code     *string
	Description   string
	DiagnosisType string
	CreatedAt     time.Time
}

type DiagnosisInput struct {
	ICD10Code     *string
	Description   string
	DiagnosisType string
}

// SearchICD10 backs the htmx autocomplete for diagnosis codes.
func (s *Store) SearchICD10(ctx context.Context, q string) ([]ICD10Code, error) {
	pattern := "%" + q + "%"
	rows, err := s.Pool.Query(ctx,
		`SELECT code, description FROM icd10_codes
		 WHERE code ILIKE $1 OR description ILIKE $1
		 ORDER BY code LIMIT 15`, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ICD10Code
	for rows.Next() {
		var c ICD10Code
		if err := rows.Scan(&c.Code, &c.Description); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) ListDiagnoses(ctx context.Context, encounterID int64) ([]Diagnosis, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, encounter_id, icd10_code, description, diagnosis_type, created_at
		 FROM diagnoses WHERE encounter_id = $1 ORDER BY created_at`, encounterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Diagnosis
	for rows.Next() {
		var d Diagnosis
		if err := rows.Scan(&d.ID, &d.EncounterID, &d.ICD10Code, &d.Description, &d.DiagnosisType, &d.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) AddDiagnosis(ctx context.Context, encounterID int64, in DiagnosisInput) error {
	tag, err := s.Pool.Exec(ctx, `
		INSERT INTO diagnoses (encounter_id, icd10_code, description, diagnosis_type)
		SELECT $1, $2, $3, $4
		WHERE EXISTS (SELECT 1 FROM encounters WHERE id = $1 AND status = 'in_progress')`,
		encounterID, in.ICD10Code, in.Description, in.DiagnosisType)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCompleted
	}
	return nil
}

func (s *Store) DeleteDiagnosis(ctx context.Context, id, encounterID int64) error {
	tag, err := s.Pool.Exec(ctx, `
		DELETE FROM diagnoses d USING encounters e
		WHERE d.id = $1 AND d.encounter_id = $2 AND e.id = d.encounter_id
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
