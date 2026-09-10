package encounter

import (
	"context"
	"time"
)

type PrescriptionItem struct {
	ID             int64
	PrescriptionID int64
	DrugID         *int64
	DrugName       string
	DosageID       *int64
	DosageText     string
	DurationID     *int64
	DurationText   string
	Quantity       *float64
	Remark         string
	CreatedAt      time.Time
}

// PrescriptionItemInput accepts either a catalog link (DrugID/DosageID/
// DurationID) or free text, per field independently. DrugName is always
// populated so the prescription stays readable even if the catalog entry
// is later renamed.
type PrescriptionItemInput struct {
	DrugID       *int64
	DrugName     string
	DosageID     *int64
	DosageText   string
	DurationID   *int64
	DurationText string
	Quantity     *float64
	Remark       string
}

func (s *Store) ListPrescriptionItems(ctx context.Context, encounterID int64) ([]PrescriptionItem, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT pi.id, pi.prescription_id, pi.drug_id, pi.drug_name, pi.dosage_id, pi.dosage_text,
			pi.duration_id, pi.duration_text, pi.quantity, pi.remark, pi.created_at
		FROM prescription_items pi
		JOIN prescriptions p ON p.id = pi.prescription_id
		WHERE p.encounter_id = $1
		ORDER BY pi.created_at`, encounterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PrescriptionItem
	for rows.Next() {
		var it PrescriptionItem
		if err := rows.Scan(&it.ID, &it.PrescriptionID, &it.DrugID, &it.DrugName, &it.DosageID,
			&it.DosageText, &it.DurationID, &it.DurationText, &it.Quantity, &it.Remark, &it.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) AddPrescriptionItem(ctx context.Context, encounterID int64, in PrescriptionItemInput) error {
	var prescriptionID int64
	err := s.Pool.QueryRow(ctx, `
		SELECT p.id FROM prescriptions p
		JOIN encounters e ON e.id = p.encounter_id
		WHERE p.encounter_id = $1 AND e.status = 'in_progress'`,
		encounterID).Scan(&prescriptionID)
	if err != nil {
		return ErrCompleted
	}

	_, err = s.Pool.Exec(ctx, `
		INSERT INTO prescription_items (prescription_id, drug_id, drug_name, dosage_id, dosage_text,
			duration_id, duration_text, quantity, remark)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		prescriptionID, in.DrugID, in.DrugName, in.DosageID, in.DosageText,
		in.DurationID, in.DurationText, in.Quantity, in.Remark)
	return err
}

func (s *Store) DeletePrescriptionItem(ctx context.Context, id, encounterID int64) error {
	tag, err := s.Pool.Exec(ctx, `
		DELETE FROM prescription_items pi
		USING prescriptions p, encounters e
		WHERE pi.id = $1 AND pi.prescription_id = p.id AND p.encounter_id = $2
		  AND e.id = p.encounter_id AND e.status = 'in_progress'`,
		id, encounterID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCompleted
	}
	return nil
}

// DispenseStock decrements drugs.stock_qty for every prescription item with
// a linked drug_id and quantity. Skips items already dispensed, so it's
// safe to call again after a Reopen + re-Complete cycle.
func (s *Store) DispenseStock(ctx context.Context, encounterID, createdBy int64) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT pi.id, pi.drug_id, pi.quantity
		FROM prescription_items pi
		JOIN prescriptions p ON p.id = pi.prescription_id
		WHERE p.encounter_id = $1 AND pi.drug_id IS NOT NULL AND pi.quantity IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM stock_movements sm WHERE sm.reason = 'dispense' AND sm.reference_id = pi.id
		  )`,
		encounterID)
	if err != nil {
		return err
	}
	type dispenseRow struct {
		itemID   int64
		drugID   int64
		quantity float64
	}
	var toDispense []dispenseRow
	for rows.Next() {
		var d dispenseRow
		if err := rows.Scan(&d.itemID, &d.drugID, &d.quantity); err != nil {
			rows.Close()
			return err
		}
		toDispense = append(toDispense, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, d := range toDispense {
		if _, err := tx.Exec(ctx,
			`INSERT INTO stock_movements (drug_id, change_qty, reason, reference_id, created_by)
			 VALUES ($1,$2,'dispense',$3,$4)`,
			d.drugID, -d.quantity, d.itemID, createdBy); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE drugs SET stock_qty = stock_qty - $2 WHERE id = $1`, d.drugID, d.quantity); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}
