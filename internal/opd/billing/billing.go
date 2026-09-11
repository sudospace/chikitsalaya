// Package billing implements itemized GST invoicing (with HSN/SAC and
// CGST/SGST) and a multi-payment ledger per invoice.
package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrAlreadyPaid is returned by Cancel when the invoice is already fully paid.
var ErrAlreadyPaid = errors.New("this invoice is already paid and can't be cancelled")

type Invoice struct {
	ID               int64
	InvoiceNumber    string
	PatientID        int64
	PatientName      string
	PractitionerID   int64
	PractitionerName string
	AppointmentID    *int64
	EncounterID      *int64
	Status           string
	Subtotal         float64
	DiscountAmount   float64
	CGSTAmount       float64
	SGSTAmount       float64
	Total            float64
	AmountPaid       float64
	Balance          float64
	Notes            string
	IssuedAt         time.Time
	CreatedBy        *int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type LineItem struct {
	ID                  int64
	InvoiceID           int64
	Kind                string
	Description         string
	HSNSACCode          string
	Quantity            float64
	UnitPrice           float64
	Amount              float64
	TaxRate             float64
	TaxAmount           float64
	LabOrderID          *int64
	ProcedureOrderID    *int64
	PrescriptionItemID  *int64
	SortOrder           int
}

type Payment struct {
	ID             int64
	InvoiceID      int64
	Amount         float64
	Method         string
	ReferenceNo    string
	Notes          string
	PaidAt         time.Time
	RecordedBy     *int64
	RecordedByName string
	CreatedAt      time.Time
}

// LineItemInput is one row of a CreateInvoiceInput — Amount/TaxAmount are
// computed server-side from Quantity/UnitPrice/TaxRate, never trusted from
// the client.
type LineItemInput struct {
	Kind               string
	Description        string
	HSNSACCode         string
	Quantity           float64
	UnitPrice          float64
	TaxRate            float64
	LabOrderID         *int64
	ProcedureOrderID   *int64
	PrescriptionItemID *int64
}

// CreateInvoiceInput's AppointmentID/EncounterID are both nil for the
// standalone quick-invoice path (no visit to tie back to).
type CreateInvoiceInput struct {
	PatientID      int64
	PractitionerID int64
	AppointmentID  *int64
	EncounterID    *int64
	DiscountAmount float64
	Notes          string
	CreatedBy      *int64
	Items          []LineItemInput
}

type PaymentInput struct {
	Amount      float64
	Method      string
	ReferenceNo string
	Notes       string
	PaidAt      time.Time
	RecordedBy  *int64
}

// ListFilter's zero-value fields are simply omitted from the WHERE clause.
type ListFilter struct {
	Status         string
	PatientID      *int64
	PractitionerID *int64
	From, To       *time.Time
}

type Store struct {
	Pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool}
}

const invoiceSelectBase = `SELECT i.id, i.invoice_number, i.patient_id, p.first_name || ' ' || p.last_name,
	i.practitioner_id, pr.full_name, i.appointment_id, i.encounter_id, i.status,
	i.subtotal, i.discount_amount, i.cgst_amount, i.sgst_amount, i.total,
	COALESCE((SELECT SUM(pay.amount) FROM payments pay WHERE pay.invoice_id = i.id), 0),
	i.notes, i.issued_at, i.created_by, i.created_at, i.updated_at
	FROM invoices i
	JOIN patients p ON p.id = i.patient_id
	JOIN practitioners pr ON pr.id = i.practitioner_id`

func scanInvoice(row pgx.Row) (*Invoice, error) {
	var inv Invoice
	err := row.Scan(&inv.ID, &inv.InvoiceNumber, &inv.PatientID, &inv.PatientName,
		&inv.PractitionerID, &inv.PractitionerName, &inv.AppointmentID, &inv.EncounterID, &inv.Status,
		&inv.Subtotal, &inv.DiscountAmount, &inv.CGSTAmount, &inv.SGSTAmount, &inv.Total, &inv.AmountPaid,
		&inv.Notes, &inv.IssuedAt, &inv.CreatedBy, &inv.CreatedAt, &inv.UpdatedAt)
	if err != nil {
		return nil, err
	}
	inv.Balance = inv.Total - inv.AmountPaid
	return &inv, nil
}

// Create reserves the invoice id (same trick patient.Store.Create uses for
// MRNs), computes line amounts/tax and invoice totals server-side, and
// inserts everything in one transaction. GST is zeroed out entirely when
// the clinic isn't gst_registered, regardless of any line's tax_rate.
func (s *Store) Create(ctx context.Context, in CreateInvoiceInput) (int64, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var id int64
	if err := tx.QueryRow(ctx,
		`SELECT nextval(pg_get_serial_sequence('invoices', 'id'))`).Scan(&id); err != nil {
		return 0, fmt.Errorf("reserve id: %w", err)
	}
	invoiceNumber := fmt.Sprintf("INV-%06d", id)

	var gstRegistered bool
	if err := tx.QueryRow(ctx, `SELECT gst_registered FROM clinic_settings LIMIT 1`).Scan(&gstRegistered); err != nil {
		return 0, fmt.Errorf("load gst setting: %w", err)
	}

	// Compute every line's amount/tax first (no writes yet) so the invoice
	// row — which invoice_line_items.invoice_id must reference — can be
	// inserted before any line item, not after.
	type computedLine struct {
		item      LineItemInput
		amount    float64
		taxAmount float64
	}
	lines := make([]computedLine, len(in.Items))
	var subtotal, totalTax float64
	for i, item := range in.Items {
		amount := item.Quantity * item.UnitPrice
		taxAmount := 0.0
		if gstRegistered {
			taxAmount = amount * item.TaxRate / 100
		}
		subtotal += amount
		totalTax += taxAmount
		lines[i] = computedLine{item: item, amount: amount, taxAmount: taxAmount}
	}

	cgst := totalTax / 2
	sgst := totalTax - cgst
	total := subtotal - in.DiscountAmount + cgst + sgst

	if _, err := tx.Exec(ctx, `
		INSERT INTO invoices (id, invoice_number, patient_id, practitioner_id, appointment_id,
			encounter_id, subtotal, discount_amount, cgst_amount, sgst_amount, total, notes, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		id, invoiceNumber, in.PatientID, in.PractitionerID, in.AppointmentID, in.EncounterID,
		subtotal, in.DiscountAmount, cgst, sgst, total, in.Notes, in.CreatedBy); err != nil {
		return 0, err
	}

	for i, l := range lines {
		if _, err := tx.Exec(ctx, `
			INSERT INTO invoice_line_items (invoice_id, kind, description, hsn_sac_code, quantity,
				unit_price, amount, tax_rate, tax_amount, lab_order_id, procedure_order_id,
				prescription_item_id, sort_order)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
			id, l.item.Kind, l.item.Description, l.item.HSNSACCode, l.item.Quantity, l.item.UnitPrice,
			l.amount, l.item.TaxRate, l.taxAmount, l.item.LabOrderID, l.item.ProcedureOrderID,
			l.item.PrescriptionItemID, i); err != nil {
			return 0, err
		}
	}

	return id, tx.Commit(ctx)
}

func (s *Store) Get(ctx context.Context, id int64) (*Invoice, error) {
	row := s.Pool.QueryRow(ctx, invoiceSelectBase+` WHERE i.id = $1`, id)
	return scanInvoice(row)
}

func (s *Store) ListLineItems(ctx context.Context, invoiceID int64) ([]LineItem, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, invoice_id, kind, description, hsn_sac_code, quantity, unit_price, amount,
			tax_rate, tax_amount, lab_order_id, procedure_order_id, prescription_item_id, sort_order
		FROM invoice_line_items WHERE invoice_id = $1 ORDER BY sort_order, id`, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LineItem
	for rows.Next() {
		var li LineItem
		if err := rows.Scan(&li.ID, &li.InvoiceID, &li.Kind, &li.Description, &li.HSNSACCode,
			&li.Quantity, &li.UnitPrice, &li.Amount, &li.TaxRate, &li.TaxAmount,
			&li.LabOrderID, &li.ProcedureOrderID, &li.PrescriptionItemID, &li.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, li)
	}
	return out, rows.Err()
}

// List applies filter's non-zero fields as additional WHERE clauses.
func (s *Store) List(ctx context.Context, filter ListFilter) ([]Invoice, error) {
	query := invoiceSelectBase + ` WHERE 1=1`
	var args []any

	if filter.Status != "" {
		args = append(args, filter.Status)
		query += fmt.Sprintf(" AND i.status = $%d", len(args))
	}
	if filter.PatientID != nil {
		args = append(args, *filter.PatientID)
		query += fmt.Sprintf(" AND i.patient_id = $%d", len(args))
	}
	if filter.PractitionerID != nil {
		args = append(args, *filter.PractitionerID)
		query += fmt.Sprintf(" AND i.practitioner_id = $%d", len(args))
	}
	if filter.From != nil {
		args = append(args, *filter.From)
		query += fmt.Sprintf(" AND i.issued_at >= $%d", len(args))
	}
	if filter.To != nil {
		args = append(args, *filter.To)
		query += fmt.Sprintf(" AND i.issued_at < $%d", len(args))
	}
	query += ` ORDER BY i.issued_at DESC`

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Invoice
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inv)
	}
	return out, rows.Err()
}

// RecordPayment inserts the payment and recomputes invoices.status from the
// running SUM(payments.amount) vs. total. A cancelled invoice is never
// touched by this, even if a payment somehow gets recorded against it.
func (s *Store) RecordPayment(ctx context.Context, invoiceID int64, in PaymentInput) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO payments (invoice_id, amount, method, reference_no, notes, paid_at, recorded_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		invoiceID, in.Amount, in.Method, in.ReferenceNo, in.Notes, in.PaidAt, in.RecordedBy); err != nil {
		return err
	}

	var total, paid float64
	if err := tx.QueryRow(ctx, `SELECT total FROM invoices WHERE id = $1`, invoiceID).Scan(&total); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM payments WHERE invoice_id = $1`, invoiceID).Scan(&paid); err != nil {
		return err
	}

	status := "unpaid"
	switch {
	case paid >= total && total > 0:
		status = "paid"
	case paid > 0:
		status = "partially_paid"
	}

	if _, err := tx.Exec(ctx,
		`UPDATE invoices SET status = $2, updated_at = now() WHERE id = $1 AND status != 'cancelled'`,
		invoiceID, status); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (s *Store) ListPayments(ctx context.Context, invoiceID int64) ([]Payment, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT pay.id, pay.invoice_id, pay.amount, pay.method, pay.reference_no, pay.notes,
			pay.paid_at, pay.recorded_by, COALESCE(NULLIF(u.full_name, ''), u.email, ''), pay.created_at
		FROM payments pay
		LEFT JOIN users u ON u.id = pay.recorded_by
		WHERE pay.invoice_id = $1 ORDER BY pay.paid_at DESC`, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Payment
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.InvoiceID, &p.Amount, &p.Method, &p.ReferenceNo, &p.Notes,
			&p.PaidAt, &p.RecordedBy, &p.RecordedByName, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Cancel refuses to cancel an invoice that's already fully paid.
func (s *Store) Cancel(ctx context.Context, id int64) error {
	tag, err := s.Pool.Exec(ctx,
		`UPDATE invoices SET status = 'cancelled', updated_at = now() WHERE id = $1 AND status != 'paid'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrAlreadyPaid
	}
	return nil
}
