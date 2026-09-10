// Package masters implements the small reference catalogs prescriptions
// draw from: dosage/duration picklists and lab/procedure order templates.
package masters

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LookupCategories are the fixed picklist categories lookup_values serves.
var LookupCategories = []string{"dosage", "duration"}

type LookupValue struct {
	ID        int64
	Category  string
	Value     string
	SortOrder int
}

type LookupValueInput struct {
	Category  string
	Value     string
	SortOrder int
}

type LabTestTemplate struct {
	ID         int64
	Name       string
	Department string
	Price      *float64
	SampleType string
	IsActive   bool
	CreatedAt  time.Time
}

type LabTestTemplateInput struct {
	Name       string
	Department string
	Price      *float64
	SampleType string
	IsActive   bool
}

type ProcedureTemplate struct {
	ID          int64
	Name        string
	Department  string
	Price       *float64
	Description string
	IsActive    bool
	CreatedAt   time.Time
}

type ProcedureTemplateInput struct {
	Name        string
	Department  string
	Price       *float64
	Description string
	IsActive    bool
}

type Store struct {
	Pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool}
}

// --- Lookup values ---

func (s *Store) ListLookupValues(ctx context.Context) ([]LookupValue, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, category, value, sort_order FROM lookup_values ORDER BY category, sort_order, value`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LookupValue
	for rows.Next() {
		var v LookupValue
		if err := rows.Scan(&v.ID, &v.Category, &v.Value, &v.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ListLookupByCategory is used elsewhere (prescription writing) to
// populate one picklist, e.g. all "dosage" values in display order.
func (s *Store) ListLookupByCategory(ctx context.Context, category string) ([]LookupValue, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, category, value, sort_order FROM lookup_values WHERE category = $1 ORDER BY sort_order, value`, category)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LookupValue
	for rows.Next() {
		var v LookupValue
		if err := rows.Scan(&v.ID, &v.Category, &v.Value, &v.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) GetLookupValue(ctx context.Context, id int64) (*LookupValue, error) {
	var v LookupValue
	err := s.Pool.QueryRow(ctx,
		`SELECT id, category, value, sort_order FROM lookup_values WHERE id = $1`, id).
		Scan(&v.ID, &v.Category, &v.Value, &v.SortOrder)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (s *Store) CreateLookupValue(ctx context.Context, in LookupValueInput) (int64, error) {
	var id int64
	err := s.Pool.QueryRow(ctx,
		`INSERT INTO lookup_values (category, value, sort_order) VALUES ($1,$2,$3) RETURNING id`,
		in.Category, in.Value, in.SortOrder).Scan(&id)
	return id, err
}

func (s *Store) UpdateLookupValue(ctx context.Context, id int64, in LookupValueInput) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE lookup_values SET category = $2, value = $3, sort_order = $4 WHERE id = $1`,
		id, in.Category, in.Value, in.SortOrder)
	return err
}

func (s *Store) DeleteLookupValue(ctx context.Context, id int64) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM lookup_values WHERE id = $1`, id)
	return err
}

// FindLookupValueByName matches case-insensitively within one category
// (dosage/duration share the same table), same purpose as
// FindLabTestTemplateByName.
func (s *Store) FindLookupValueByName(ctx context.Context, category, value string) (*LookupValue, error) {
	var v LookupValue
	err := s.Pool.QueryRow(ctx,
		`SELECT id, category, value, sort_order FROM lookup_values
		 WHERE category = $1 AND lower(value) = lower($2) LIMIT 1`,
		category, strings.TrimSpace(value)).Scan(&v.ID, &v.Category, &v.Value, &v.SortOrder)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// --- Lab test templates ---

const labTestColumns = `id, name, department, price, sample_type, is_active, created_at`

func (s *Store) ListLabTestTemplates(ctx context.Context) ([]LabTestTemplate, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+labTestColumns+` FROM lab_test_templates ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LabTestTemplate
	for rows.Next() {
		t, err := scanLabTestTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (s *Store) GetLabTestTemplate(ctx context.Context, id int64) (*LabTestTemplate, error) {
	row := s.Pool.QueryRow(ctx, `SELECT `+labTestColumns+` FROM lab_test_templates WHERE id = $1`, id)
	return scanLabTestTemplate(row)
}

func scanLabTestTemplate(row pgx.Row) (*LabTestTemplate, error) {
	var t LabTestTemplate
	err := row.Scan(&t.ID, &t.Name, &t.Department, &t.Price, &t.SampleType, &t.IsActive, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// FindLabTestTemplateByName matches case-insensitively, same purpose as
// inventory.Store.FindDrugByName.
func (s *Store) FindLabTestTemplateByName(ctx context.Context, name string) (*LabTestTemplate, error) {
	row := s.Pool.QueryRow(ctx,
		`SELECT `+labTestColumns+` FROM lab_test_templates WHERE lower(name) = lower($1) LIMIT 1`,
		strings.TrimSpace(name))
	return scanLabTestTemplate(row)
}

// SearchLabTestTemplates backs the live search picker, same approach as
// inventory.Store.SearchDrugs.
func (s *Store) SearchLabTestTemplates(ctx context.Context, q string) ([]LabTestTemplate, error) {
	all, err := s.ListLabTestTemplates(ctx)
	if err != nil {
		return nil, err
	}
	q = strings.ToLower(strings.TrimSpace(q))
	var out []LabTestTemplate
	for _, t := range all {
		if !t.IsActive {
			continue
		}
		if strings.Contains(strings.ToLower(t.Name), q) {
			out = append(out, t)
			if len(out) >= 20 {
				break
			}
		}
	}
	return out, nil
}

func (s *Store) CreateLabTestTemplate(ctx context.Context, in LabTestTemplateInput) (int64, error) {
	var id int64
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO lab_test_templates (name, department, price, sample_type, is_active)
		VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		in.Name, in.Department, in.Price, in.SampleType, in.IsActive).Scan(&id)
	return id, err
}

func (s *Store) UpdateLabTestTemplate(ctx context.Context, id int64, in LabTestTemplateInput) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE lab_test_templates SET name = $2, department = $3, price = $4, sample_type = $5, is_active = $6
		WHERE id = $1`,
		id, in.Name, in.Department, in.Price, in.SampleType, in.IsActive)
	return err
}

// --- Procedure templates ---

const procedureColumns = `id, name, department, price, description, is_active, created_at`

func (s *Store) ListProcedureTemplates(ctx context.Context) ([]ProcedureTemplate, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+procedureColumns+` FROM procedure_templates ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProcedureTemplate
	for rows.Next() {
		t, err := scanProcedureTemplate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func (s *Store) GetProcedureTemplate(ctx context.Context, id int64) (*ProcedureTemplate, error) {
	row := s.Pool.QueryRow(ctx, `SELECT `+procedureColumns+` FROM procedure_templates WHERE id = $1`, id)
	return scanProcedureTemplate(row)
}

func scanProcedureTemplate(row pgx.Row) (*ProcedureTemplate, error) {
	var t ProcedureTemplate
	err := row.Scan(&t.ID, &t.Name, &t.Department, &t.Price, &t.Description, &t.IsActive, &t.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// SearchProcedureTemplates backs the live search picker, same approach as
// SearchLabTestTemplates.
func (s *Store) SearchProcedureTemplates(ctx context.Context, q string) ([]ProcedureTemplate, error) {
	all, err := s.ListProcedureTemplates(ctx)
	if err != nil {
		return nil, err
	}
	q = strings.ToLower(strings.TrimSpace(q))
	var out []ProcedureTemplate
	for _, t := range all {
		if !t.IsActive {
			continue
		}
		if strings.Contains(strings.ToLower(t.Name), q) {
			out = append(out, t)
			if len(out) >= 20 {
				break
			}
		}
	}
	return out, nil
}

// FindProcedureTemplateByName matches case-insensitively, same purpose as
// FindLabTestTemplateByName.
func (s *Store) FindProcedureTemplateByName(ctx context.Context, name string) (*ProcedureTemplate, error) {
	row := s.Pool.QueryRow(ctx,
		`SELECT `+procedureColumns+` FROM procedure_templates WHERE lower(name) = lower($1) LIMIT 1`,
		strings.TrimSpace(name))
	return scanProcedureTemplate(row)
}

func (s *Store) CreateProcedureTemplate(ctx context.Context, in ProcedureTemplateInput) (int64, error) {
	var id int64
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO procedure_templates (name, department, price, description, is_active)
		VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		in.Name, in.Department, in.Price, in.Description, in.IsActive).Scan(&id)
	return id, err
}

func (s *Store) UpdateProcedureTemplate(ctx context.Context, id int64, in ProcedureTemplateInput) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE procedure_templates SET name = $2, department = $3, price = $4, description = $5, is_active = $6
		WHERE id = $1`,
		id, in.Name, in.Department, in.Price, in.Description, in.IsActive)
	return err
}
