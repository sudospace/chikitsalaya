// Package practitioner implements healthcare practitioner records.
package practitioner

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Practitioner struct {
	ID             int64
	FullName       string
	Qualification  string
	Specialization string
	Phone          string
	Email          string
	IsActive       bool
	CreatedAt      time.Time
}

type Input struct {
	FullName       string
	Qualification  string
	Specialization string
	Phone          string
	Email          string
	IsActive       bool
}

type Store struct {
	Pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool}
}

const selectBase = `SELECT p.id, p.full_name, p.qualification, p.specialization,
	p.phone, p.email, p.is_active, p.created_at
	FROM practitioners p`

func (s *Store) List(ctx context.Context) ([]Practitioner, error) {
	rows, err := s.Pool.Query(ctx, selectBase+` ORDER BY p.full_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Practitioner
	for rows.Next() {
		p, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (s *Store) Get(ctx context.Context, id int64) (*Practitioner, error) {
	row := s.Pool.QueryRow(ctx, selectBase+` WHERE p.id = $1`, id)
	return scan(row)
}

func scan(row pgx.Row) (*Practitioner, error) {
	var p Practitioner
	err := row.Scan(&p.ID, &p.FullName, &p.Qualification,
		&p.Specialization, &p.Phone, &p.Email, &p.IsActive, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) Create(ctx context.Context, in Input) (int64, error) {
	var id int64
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO practitioners (full_name, qualification, specialization, phone, email, is_active)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		in.FullName, in.Qualification, in.Specialization, in.Phone, in.Email, in.IsActive).Scan(&id)
	return id, err
}

func (s *Store) Update(ctx context.Context, id int64, in Input) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE practitioners SET
			full_name = $2, qualification = $3, specialization = $4,
			phone = $5, email = $6, is_active = $7
		WHERE id = $1`,
		id, in.FullName, in.Qualification, in.Specialization, in.Phone, in.Email, in.IsActive)
	return err
}
