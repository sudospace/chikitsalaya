// Package inventory implements the drug catalog and its append-only stock
// ledger — stock_qty only ever changes alongside a stock_movements row.
package inventory

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Drug struct {
	ID           int64
	Name         string
	GenericName  string
	Form         string
	Strength     string
	Unit         string
	StockQty     float64
	ReorderLevel *float64
	IsActive     bool
	CreatedAt    time.Time
}

// DrugInput's OpeningStock only seeds CreateDrug's initial stock_movements
// row — UpdateDrug ignores it; later stock changes must go through a movement.
type DrugInput struct {
	Name         string
	GenericName  string
	Form         string
	Strength     string
	Unit         string
	ReorderLevel *float64
	IsActive     bool
	OpeningStock float64
}

type Store struct {
	Pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool}
}

const drugColumns = `id, name, generic_name, form, strength, unit, stock_qty, reorder_level, is_active, created_at`

func (s *Store) ListDrugs(ctx context.Context) ([]Drug, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+drugColumns+` FROM drugs ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Drug
	for rows.Next() {
		d, err := scanDrug(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

func (s *Store) GetDrug(ctx context.Context, id int64) (*Drug, error) {
	row := s.Pool.QueryRow(ctx, `SELECT `+drugColumns+` FROM drugs WHERE id = $1`, id)
	return scanDrug(row)
}

// FindDrugByName matches case-insensitively so near-duplicate free text
// ("Dolo" / "dolo") reuses the same catalog row instead of spawning a new one.
func (s *Store) FindDrugByName(ctx context.Context, name string) (*Drug, error) {
	row := s.Pool.QueryRow(ctx,
		`SELECT `+drugColumns+` FROM drugs WHERE lower(name) = lower($1) LIMIT 1`,
		strings.TrimSpace(name))
	return scanDrug(row)
}

// SearchDrugs backs the live search-as-you-type picker — filters the already-loaded
// list in Go rather than a new query, since the catalog is small.
func (s *Store) SearchDrugs(ctx context.Context, q string) ([]Drug, error) {
	all, err := s.ListDrugs(ctx)
	if err != nil {
		return nil, err
	}
	q = strings.ToLower(strings.TrimSpace(q))
	var out []Drug
	for _, d := range all {
		if !d.IsActive {
			continue
		}
		if strings.Contains(strings.ToLower(d.Name), q) || strings.Contains(strings.ToLower(d.GenericName), q) {
			out = append(out, d)
			if len(out) >= 20 {
				break
			}
		}
	}
	return out, nil
}

func scanDrug(row pgx.Row) (*Drug, error) {
	var d Drug
	err := row.Scan(&d.ID, &d.Name, &d.GenericName, &d.Form, &d.Strength, &d.Unit,
		&d.StockQty, &d.ReorderLevel, &d.IsActive, &d.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// CreateDrug inserts the catalog row and, if OpeningStock is non-zero,
// records it as an 'opening' stock movement in the same transaction.
func (s *Store) CreateDrug(ctx context.Context, in DrugInput, createdBy int64) (int64, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO drugs (name, generic_name, form, strength, unit, stock_qty, reorder_level, is_active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		in.Name, in.GenericName, in.Form, in.Strength, in.Unit, in.OpeningStock, in.ReorderLevel, in.IsActive).Scan(&id)
	if err != nil {
		return 0, err
	}

	if in.OpeningStock != 0 {
		_, err = tx.Exec(ctx,
			`INSERT INTO stock_movements (drug_id, change_qty, reason, created_by) VALUES ($1,$2,'opening',$3)`,
			id, in.OpeningStock, createdBy)
		if err != nil {
			return 0, err
		}
	}

	return id, tx.Commit(ctx)
}

func (s *Store) UpdateDrug(ctx context.Context, id int64, in DrugInput) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE drugs SET name = $2, generic_name = $3, form = $4, strength = $5, unit = $6,
			reorder_level = $7, is_active = $8
		WHERE id = $1`,
		id, in.Name, in.GenericName, in.Form, in.Strength, in.Unit, in.ReorderLevel, in.IsActive)
	return err
}
