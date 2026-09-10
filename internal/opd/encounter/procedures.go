package encounter

import (
	"context"
	"time"
)

type ProcedureOrder struct {
	ID                  int64
	EncounterID         int64
	ProcedureTemplateID *int64
	ProcedureName       string
	Comments            string
	Status              string
	OrderedAt           time.Time
}

type ProcedureOrderInput struct {
	ProcedureTemplateID *int64
	ProcedureName       string
	Comments            string
}

func (s *Store) ListProcedureOrders(ctx context.Context, encounterID int64) ([]ProcedureOrder, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, encounter_id, procedure_template_id, procedure_name, comments, status, ordered_at
		FROM procedure_orders WHERE encounter_id = $1 ORDER BY ordered_at`, encounterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProcedureOrder
	for rows.Next() {
		var po ProcedureOrder
		if err := rows.Scan(&po.ID, &po.EncounterID, &po.ProcedureTemplateID, &po.ProcedureName,
			&po.Comments, &po.Status, &po.OrderedAt); err != nil {
			return nil, err
		}
		out = append(out, po)
	}
	return out, rows.Err()
}

func (s *Store) AddProcedureOrder(ctx context.Context, encounterID, patientID int64, in ProcedureOrderInput) error {
	tag, err := s.Pool.Exec(ctx, `
		INSERT INTO procedure_orders (encounter_id, patient_id, procedure_template_id, procedure_name, comments)
		SELECT $1, $2, $3, $4, $5
		WHERE EXISTS (SELECT 1 FROM encounters WHERE id = $1 AND status = 'in_progress')`,
		encounterID, patientID, in.ProcedureTemplateID, in.ProcedureName, in.Comments)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCompleted
	}
	return nil
}

func (s *Store) DeleteProcedureOrder(ctx context.Context, id, encounterID int64) error {
	tag, err := s.Pool.Exec(ctx, `
		DELETE FROM procedure_orders po USING encounters e
		WHERE po.id = $1 AND po.encounter_id = $2 AND e.id = po.encounter_id
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
