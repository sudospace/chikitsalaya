// Package document implements patient-scoped file attachments (lab/scan
// reports, any file type) stored through the pluggable internal/storage
// backend.
package document

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Document struct {
	ID               int64
	PatientID        int64
	OriginalFilename string
	StorageBackend   string
	StorageKey       string
	ContentType      string
	SizeBytes        int64
	Description      string
	UploadedBy       *int64
	UploadedByName   string
	UploadedAt       time.Time
}

// HumanSize renders SizeBytes as e.g. "48 KB"/"3.1 MB" -- html/template has
// no arithmetic of its own, so this lives here rather than in the template.
func (d Document) HumanSize() string {
	const kb, mb = 1024, 1024 * 1024
	switch {
	case d.SizeBytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(d.SizeBytes)/mb)
	case d.SizeBytes >= kb:
		return fmt.Sprintf("%.0f KB", float64(d.SizeBytes)/kb)
	default:
		return fmt.Sprintf("%d B", d.SizeBytes)
	}
}

type Store struct {
	Pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool}
}

const selectBase = `SELECT d.id, d.patient_id, d.original_filename, d.storage_backend, d.storage_key,
	d.content_type, d.size_bytes, d.description, d.uploaded_by,
	COALESCE(NULLIF(u.full_name, ''), u.email, ''), d.uploaded_at
	FROM patient_documents d
	LEFT JOIN users u ON u.id = d.uploaded_by`

func scanDocument(row pgx.Row) (*Document, error) {
	var d Document
	err := row.Scan(&d.ID, &d.PatientID, &d.OriginalFilename, &d.StorageBackend, &d.StorageKey,
		&d.ContentType, &d.SizeBytes, &d.Description, &d.UploadedBy, &d.UploadedByName, &d.UploadedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) ListByPatient(ctx context.Context, patientID int64) ([]Document, error) {
	rows, err := s.Pool.Query(ctx, selectBase+` WHERE d.patient_id = $1 ORDER BY d.uploaded_at DESC`, patientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Document
	for rows.Next() {
		d, err := scanDocument(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

func (s *Store) Get(ctx context.Context, id int64) (*Document, error) {
	row := s.Pool.QueryRow(ctx, selectBase+` WHERE d.id = $1`, id)
	return scanDocument(row)
}

// Create inserts a row already fully resolved (StorageKey included) — the
// handler picks the key (it doesn't depend on the DB-assigned id) before
// calling this, so there's no need to reserve an id ahead of time the way
// patient.Store.Create/billing.Store.Create do for their own id-derived keys.
func (s *Store) Create(ctx context.Context, in Document) (int64, error) {
	var id int64
	err := s.Pool.QueryRow(ctx, `
		INSERT INTO patient_documents (patient_id, original_filename, storage_backend, storage_key,
			content_type, size_bytes, description, uploaded_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id`,
		in.PatientID, in.OriginalFilename, in.StorageBackend, in.StorageKey,
		in.ContentType, in.SizeBytes, in.Description, in.UploadedBy).Scan(&id)
	return id, err
}

func (s *Store) Delete(ctx context.Context, id int64) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM patient_documents WHERE id = $1`, id)
	return err
}
