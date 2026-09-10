package encounter

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type VitalSigns struct {
	ID           int64
	EncounterID  int64
	HeightCM     *float64
	WeightKG     *float64
	BMI          *float64
	TemperatureC *float64
	PulseBPM     *int
	RespRate     *int
	BPSystolic   *int
	BPDiastolic  *int
	SpO2         *int
	RecordedAt   time.Time
}

type VitalSignsInput struct {
	HeightCM     *float64
	WeightKG     *float64
	TemperatureC *float64
	PulseBPM     *int
	RespRate     *int
	BPSystolic   *int
	BPDiastolic  *int
	SpO2         *int
}

const vitalsColumns = `id, encounter_id, height_cm, weight_kg, bmi, temperature_c,
	pulse_bpm, resp_rate, bp_systolic, bp_diastolic, spo2, recorded_at`

func (s *Store) GetVitals(ctx context.Context, encounterID int64) (*VitalSigns, error) {
	row := s.Pool.QueryRow(ctx, `SELECT `+vitalsColumns+` FROM vital_signs WHERE encounter_id = $1`, encounterID)
	return scanVitals(row)
}

func scanVitals(row pgx.Row) (*VitalSigns, error) {
	var v VitalSigns
	err := row.Scan(&v.ID, &v.EncounterID, &v.HeightCM, &v.WeightKG, &v.BMI, &v.TemperatureC,
		&v.PulseBPM, &v.RespRate, &v.BPSystolic, &v.BPDiastolic, &v.SpO2, &v.RecordedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// UpsertVitals writes (or overwrites) the encounter's single vitals row,
// computing BMI server-side from height+weight when both are given.
func (s *Store) UpsertVitals(ctx context.Context, encounterID int64, in VitalSignsInput) error {
	var bmi *float64
	if in.HeightCM != nil && in.WeightKG != nil && *in.HeightCM > 0 {
		heightM := *in.HeightCM / 100
		v := *in.WeightKG / (heightM * heightM)
		bmi = &v
	}

	tag, err := s.Pool.Exec(ctx, `
		INSERT INTO vital_signs (encounter_id, height_cm, weight_kg, bmi, temperature_c,
			pulse_bpm, resp_rate, bp_systolic, bp_diastolic, spo2, recorded_at)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now()
		WHERE EXISTS (SELECT 1 FROM encounters WHERE id = $1 AND status = 'in_progress')
		ON CONFLICT (encounter_id) DO UPDATE SET
			height_cm = EXCLUDED.height_cm, weight_kg = EXCLUDED.weight_kg, bmi = EXCLUDED.bmi,
			temperature_c = EXCLUDED.temperature_c, pulse_bpm = EXCLUDED.pulse_bpm,
			resp_rate = EXCLUDED.resp_rate, bp_systolic = EXCLUDED.bp_systolic,
			bp_diastolic = EXCLUDED.bp_diastolic, spo2 = EXCLUDED.spo2, recorded_at = now()`,
		encounterID, in.HeightCM, in.WeightKG, bmi, in.TemperatureC,
		in.PulseBPM, in.RespRate, in.BPSystolic, in.BPDiastolic, in.SpO2)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrCompleted
	}
	return nil
}
