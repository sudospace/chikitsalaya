// Package schedule implements practitioners' weekly recurring availability,
// which later drives appointment slot generation.
package schedule

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Schedule is one recurring weekly availability block. DayOfWeek follows
// Go's time.Weekday convention (0=Sunday..6=Saturday) for direct comparison
// against time.Time.Weekday() during slot generation.
type Schedule struct {
	ID                  int64
	PractitionerID      int64
	DayOfWeek           int
	StartTime           string // "HH:MM"
	EndTime             string // "HH:MM"
	SlotDurationMinutes int
	ServiceUnitID       *int64
	ServiceUnitName     string
	IsActive            bool
	CreatedAt           time.Time
}

type Input struct {
	PractitionerID      int64
	DayOfWeek           int
	StartTime           string
	EndTime             string
	SlotDurationMinutes int
	ServiceUnitID       *int64
	IsActive            bool
}

type Store struct {
	Pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool}
}

const selectBase = `SELECT s.id, s.practitioner_id, s.day_of_week,
	to_char(s.start_time, 'HH24:MI'), to_char(s.end_time, 'HH24:MI'),
	s.slot_duration_minutes, s.service_unit_id, COALESCE(su.name, ''), s.is_active, s.created_at
	FROM practitioner_schedules s LEFT JOIN service_units su ON su.id = s.service_unit_id`

func (s *Store) ListByPractitioner(ctx context.Context, practitionerID int64) ([]Schedule, error) {
	rows, err := s.Pool.Query(ctx,
		selectBase+` WHERE s.practitioner_id = $1 ORDER BY s.day_of_week, s.start_time`, practitionerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Schedule
	for rows.Next() {
		sc, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *sc)
	}
	return out, rows.Err()
}

func scan(row pgx.Row) (*Schedule, error) {
	var sc Schedule
	err := row.Scan(&sc.ID, &sc.PractitionerID, &sc.DayOfWeek, &sc.StartTime, &sc.EndTime,
		&sc.SlotDurationMinutes, &sc.ServiceUnitID, &sc.ServiceUnitName, &sc.IsActive, &sc.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &sc, nil
}

func (s *Store) Create(ctx context.Context, in Input) (int64, error) {
	var id int64
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO practitioner_schedules
			(practitioner_id, day_of_week, start_time, end_time, slot_duration_minutes, service_unit_id, is_active)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		in.PractitionerID, in.DayOfWeek, in.StartTime, in.EndTime, in.SlotDurationMinutes,
		in.ServiceUnitID, in.IsActive).Scan(&id)
	return id, err
}

// Delete removes a schedule row, scoped to practitionerID so a handler
// can't delete another practitioner's schedule via a guessed id.
func (s *Store) Delete(ctx context.Context, id, practitionerID int64) error {
	_, err := s.Pool.Exec(ctx,
		`DELETE FROM practitioner_schedules WHERE id = $1 AND practitioner_id = $2`, id, practitionerID)
	return err
}
