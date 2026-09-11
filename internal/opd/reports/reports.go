// Package reports computes revenue aggregates (by practitioner, by catalog
// line item, and a raw payment ledger) over the billing package's tables —
// never the raw clinical order tables, since invoice_line_items.unit_price
// is the one true historical price record (see billing.go).
package reports

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"chikitsalaya/internal/opd/billing"
)

// DayRange returns [midnight today, midnight tomorrow) in now's location —
// same half-open convention as appointment.dayBounds.
func DayRange(now time.Time) (start, end time.Time) {
	start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return start, start.Add(24 * time.Hour)
}

// WeekRange returns [this week's Monday, next Monday) — ISO week, Monday
// start. time.Weekday has Sunday=0..Saturday=6, so Sunday needs a +7 wrap
// to land on the *previous* Monday rather than the next one.
func WeekRange(now time.Time) (start, end time.Time) {
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	offset := int(day.Weekday()) - int(time.Monday)
	if offset < 0 {
		offset += 7
	}
	start = day.AddDate(0, 0, -offset)
	end = start.AddDate(0, 0, 7)
	return
}

// MonthRange returns [1st of this month, 1st of next month).
func MonthRange(now time.Time) (start, end time.Time) {
	start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	end = start.AddDate(0, 1, 0)
	return
}

// FYRange returns [FY start, FY start + 1 year) for the financial year
// containing now, where the FY begins on the 1st of fyStartMonth. Example:
// fyStartMonth=4 (April), now=2026-02-15 -> the FY containing Feb 2026
// started 2025-04-01 (since Feb is before April) and ends 2026-04-01.
// now=2026-06-10 -> started 2026-04-01, ends 2027-04-01.
func FYRange(now time.Time, fyStartMonth int) (start, end time.Time) {
	year := now.Year()
	if int(now.Month()) < fyStartMonth {
		year--
	}
	start = time.Date(year, time.Month(fyStartMonth), 1, 0, 0, 0, 0, now.Location())
	end = start.AddDate(1, 0, 0)
	return
}

type PractitionerRevenue struct {
	PractitionerID   int64
	PractitionerName string
	InvoiceCount     int
	Billed           float64
	Collected        float64
}

type LineItemRevenue struct {
	Kind        string
	Description string
	Count       int
	Amount      float64
}

type Store struct {
	Pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool}
}

// RevenueByPractitioner reports billed (invoice totals issued in range) and
// collected (payments received in range) per practitioner. practitionerID
// nil means every doctor; non-nil hard-filters to just that one — callers
// resolve which, this package has no notion of roles/permissions. Billed
// and collected are computed via two separate queries (rather than one
// correlated subquery) and merged in Go, since a practitioner can appear in
// one result set without the other (e.g. invoiced this week, paid last
// week).
func (s *Store) RevenueByPractitioner(ctx context.Context, start, end time.Time, practitionerID *int64) ([]PractitionerRevenue, error) {
	billedQuery := `
		SELECT i.practitioner_id, pr.full_name, COUNT(*), COALESCE(SUM(i.total), 0)
		FROM invoices i JOIN practitioners pr ON pr.id = i.practitioner_id
		WHERE i.issued_at >= $1 AND i.issued_at < $2 AND i.status != 'cancelled'`
	args := []any{start, end}
	if practitionerID != nil {
		args = append(args, *practitionerID)
		billedQuery += " AND i.practitioner_id = $3"
	}
	billedQuery += " GROUP BY i.practitioner_id, pr.full_name"

	rows, err := s.Pool.Query(ctx, billedQuery, args...)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*PractitionerRevenue)
	var order []int64
	for rows.Next() {
		var pr PractitionerRevenue
		if err := rows.Scan(&pr.PractitionerID, &pr.PractitionerName, &pr.InvoiceCount, &pr.Billed); err != nil {
			rows.Close()
			return nil, err
		}
		byID[pr.PractitionerID] = &pr
		order = append(order, pr.PractitionerID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	collectedQuery := `
		SELECT i.practitioner_id, pr.full_name, COALESCE(SUM(p.amount), 0)
		FROM payments p
		JOIN invoices i ON i.id = p.invoice_id
		JOIN practitioners pr ON pr.id = i.practitioner_id
		WHERE p.paid_at >= $1 AND p.paid_at < $2 AND i.status != 'cancelled'`
	cargs := []any{start, end}
	if practitionerID != nil {
		cargs = append(cargs, *practitionerID)
		collectedQuery += " AND i.practitioner_id = $3"
	}
	collectedQuery += " GROUP BY i.practitioner_id, pr.full_name"

	crows, err := s.Pool.Query(ctx, collectedQuery, cargs...)
	if err != nil {
		return nil, err
	}
	defer crows.Close()
	for crows.Next() {
		var practID int64
		var name string
		var collected float64
		if err := crows.Scan(&practID, &name, &collected); err != nil {
			return nil, err
		}
		pr, ok := byID[practID]
		if !ok {
			pr = &PractitionerRevenue{PractitionerID: practID, PractitionerName: name}
			byID[practID] = pr
			order = append(order, practID)
		}
		pr.Collected = collected
	}
	if err := crows.Err(); err != nil {
		return nil, err
	}

	out := make([]PractitionerRevenue, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

// RevenueByLineItem breaks down invoiced amounts by catalog line, filtered
// to one kind ("lab"/"procedure"/"drug"/"consultation"/"other") or every
// kind at once when kind is "".
func (s *Store) RevenueByLineItem(ctx context.Context, start, end time.Time, practitionerID *int64, kind string) ([]LineItemRevenue, error) {
	query := `
		SELECT ili.kind, ili.description, COUNT(*), COALESCE(SUM(ili.amount), 0)
		FROM invoice_line_items ili
		JOIN invoices i ON i.id = ili.invoice_id
		WHERE i.issued_at >= $1 AND i.issued_at < $2 AND i.status != 'cancelled'`
	args := []any{start, end}

	if practitionerID != nil {
		args = append(args, *practitionerID)
		query += fmt.Sprintf(" AND i.practitioner_id = $%d", len(args))
	}
	if kind != "" {
		args = append(args, kind)
		query += fmt.Sprintf(" AND ili.kind = $%d", len(args))
	}
	query += " GROUP BY ili.kind, ili.description ORDER BY 4 DESC"

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LineItemRevenue
	for rows.Next() {
		var li LineItemRevenue
		if err := rows.Scan(&li.Kind, &li.Description, &li.Count, &li.Amount); err != nil {
			return nil, err
		}
		out = append(out, li)
	}
	return out, rows.Err()
}

// LedgerRow is one payment plus the invoice/patient context a ledger export
// actually needs — billing.Payment alone doesn't carry an invoice number or
// patient name, which would make a CSV export nearly useless on its own.
type LedgerRow struct {
	billing.Payment
	InvoiceNumber string
	PatientName   string
}

// PaymentLedger lists every payment in range (for the CSV export). Ordered
// newest first, same as billing.Store.ListPayments.
func (s *Store) PaymentLedger(ctx context.Context, start, end time.Time, practitionerID *int64) ([]LedgerRow, error) {
	query := `
		SELECT pay.id, pay.invoice_id, pay.amount, pay.method, pay.reference_no, pay.notes,
			pay.paid_at, pay.recorded_by, COALESCE(NULLIF(u.full_name, ''), u.email, ''), pay.created_at,
			i.invoice_number, p.first_name || ' ' || p.last_name
		FROM payments pay
		JOIN invoices i ON i.id = pay.invoice_id
		JOIN patients p ON p.id = i.patient_id
		LEFT JOIN users u ON u.id = pay.recorded_by
		WHERE pay.paid_at >= $1 AND pay.paid_at < $2 AND i.status != 'cancelled'`
	args := []any{start, end}
	if practitionerID != nil {
		args = append(args, *practitionerID)
		query += fmt.Sprintf(" AND i.practitioner_id = $%d", len(args))
	}
	query += " ORDER BY pay.paid_at DESC"

	rows, err := s.Pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LedgerRow
	for rows.Next() {
		var lr LedgerRow
		if err := rows.Scan(&lr.ID, &lr.InvoiceID, &lr.Amount, &lr.Method, &lr.ReferenceNo, &lr.Notes,
			&lr.PaidAt, &lr.RecordedBy, &lr.RecordedByName, &lr.CreatedAt,
			&lr.InvoiceNumber, &lr.PatientName); err != nil {
			return nil, err
		}
		out = append(out, lr)
	}
	return out, rows.Err()
}
