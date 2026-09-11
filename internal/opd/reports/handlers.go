package reports

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/alexedwards/scs/v2"

	"chikitsalaya/internal/auth"
	"chikitsalaya/internal/opd/company"
	"chikitsalaya/internal/opd/practitioner"
	"chikitsalaya/internal/web"
)

const dateLayout = "2006-01-02"

type Handlers struct {
	Store             *Store
	CompanyStore      *company.Store
	PractitionerStore *practitioner.Store
	UserStore         *auth.Store
	Sessions          *scs.SessionManager
	Renderer          *web.Renderer
}

func NewHandlers(store *Store, companyStore *company.Store, practitionerStore *practitioner.Store,
	userStore *auth.Store, sm *scs.SessionManager, renderer *web.Renderer) *Handlers {
	return &Handlers{
		Store: store, CompanyStore: companyStore, PractitionerStore: practitionerStore,
		UserStore: userStore, Sessions: sm, Renderer: renderer,
	}
}

// resolvePractitionerScope is the hard, non-bypassable rule: a doctor (or an
// admin currently acting as one) only ever sees their own numbers, no matter
// what query params are sent — the ?practitioner_id= drill-down filter is
// read ONLY for every other role. This is deliberately not permission-
// configurable; don't add a way around it.
func (h *Handlers) resolvePractitionerScope(ctx context.Context, r *http.Request) (practitionerID *int64, isDoctorScoped bool, err error) {
	role := auth.CurrentRole(h.Sessions, r)
	if role == auth.RoleDoctor || auth.IsActingAsDoctor(h.Sessions, r) {
		u, err := h.UserStore.GetByID(ctx, auth.CurrentUserID(h.Sessions, r))
		if err != nil {
			return nil, true, err
		}
		return u.PractitionerID, true, nil
	}
	if v := r.URL.Query().Get("practitioner_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			return &id, false, nil
		}
	}
	return nil, false, nil
}

// resolveRange turns the ?range= preset (or custom from/to) into concrete
// bounds, fetching the clinic's FYStartMonth only when actually needed.
func (h *Handlers) resolveRange(ctx context.Context, r *http.Request) (start, end time.Time, rangeKey string, err error) {
	now := time.Now()
	rangeKey = r.URL.Query().Get("range")
	switch rangeKey {
	case "today":
		start, end = DayRange(now)
	case "month":
		start, end = MonthRange(now)
	case "fy":
		c, cErr := h.CompanyStore.GetClinic(ctx)
		if cErr != nil {
			return time.Time{}, time.Time{}, rangeKey, cErr
		}
		start, end = FYRange(now, c.FYStartMonth)
	case "custom":
		fromStr, toStr := r.URL.Query().Get("from"), r.URL.Query().Get("to")
		from, fErr := time.ParseInLocation(dateLayout, fromStr, time.Local)
		to, tErr := time.ParseInLocation(dateLayout, toStr, time.Local)
		if fErr != nil || tErr != nil {
			// Bad/missing custom range — fall back to this week rather than 500.
			rangeKey = "week"
			start, end = WeekRange(now)
			break
		}
		start = from
		end = to.Add(24 * time.Hour)
	default:
		rangeKey = "week"
		start, end = WeekRange(now)
	}
	return start, end, rangeKey, nil
}

type reportsData struct {
	Range           string
	From, To        string // yyyy-mm-dd, for the custom-range form and the CSV link
	IsDoctorScoped  bool
	Practitioners   []practitioner.Practitioner
	SelectedPractID int64 // 0 = "all", only meaningful when !IsDoctorScoped
	ByPractitioner  []PractitionerRevenue
	ByLab           []LineItemRevenue
	ByProcedure     []LineItemRevenue
	ByDrug          []LineItemRevenue
}

func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	start, end, rangeKey, err := h.resolveRange(ctx, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	practitionerID, isDoctorScoped, err := h.resolvePractitionerScope(ctx, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	byPract, err := h.Store.RevenueByPractitioner(ctx, start, end, practitionerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	byLab, err := h.Store.RevenueByLineItem(ctx, start, end, practitionerID, "lab")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	byProcedure, err := h.Store.RevenueByLineItem(ctx, start, end, practitionerID, "procedure")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	byDrug, err := h.Store.RevenueByLineItem(ctx, start, end, practitionerID, "drug")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := reportsData{
		Range: rangeKey, From: start.Format(dateLayout), To: end.Add(-24 * time.Hour).Format(dateLayout),
		IsDoctorScoped: isDoctorScoped,
		ByPractitioner: byPract, ByLab: byLab, ByProcedure: byProcedure, ByDrug: byDrug,
	}
	if !isDoctorScoped {
		practitioners, err := h.PractitionerStore.List(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data.Practitioners = practitioners
		if practitionerID != nil {
			data.SelectedPractID = *practitionerID
		}
	}

	h.Renderer.Render(w, r, "reports_index.html", data)
}

func (h *Handlers) LedgerCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	start, end, _, err := h.resolveRange(ctx, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	practitionerID, _, err := h.resolvePractitionerScope(ctx, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rows, err := h.Store.PaymentLedger(ctx, start, end, practitionerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("payment-ledger-%s-to-%s.csv", start.Format(dateLayout), end.Add(-24*time.Hour).Format(dateLayout))
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")

	cw := csv.NewWriter(w)
	cw.Write([]string{"Date", "Invoice Number", "Patient", "Amount", "Method", "Reference", "Recorded By"})
	for _, row := range rows {
		cw.Write([]string{
			row.PaidAt.Format(dateLayout),
			row.InvoiceNumber,
			row.PatientName,
			strconv.FormatFloat(row.Amount, 'f', 2, 64),
			row.Method,
			row.ReferenceNo,
			row.RecordedByName,
		})
	}
	cw.Flush()
}
