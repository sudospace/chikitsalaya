package billing

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"

	"chikitsalaya/internal/auth"
	"chikitsalaya/internal/opd/appointment"
	"chikitsalaya/internal/opd/company"
	"chikitsalaya/internal/opd/encounter"
	"chikitsalaya/internal/opd/inventory"
	"chikitsalaya/internal/opd/masters"
	"chikitsalaya/internal/opd/patient"
	"chikitsalaya/internal/opd/practitioner"
	"chikitsalaya/internal/web"
)

type Handlers struct {
	Store             *Store
	EncounterStore    *encounter.Store
	AppointmentStore  *appointment.Store
	PatientStore      *patient.Store
	PractitionerStore *practitioner.Store
	CompanyStore      *company.Store
	MastersStore      *masters.Store
	InventoryStore    *inventory.Store
	UserStore         *auth.Store
	Sessions          *scs.SessionManager
	Renderer          *web.Renderer
}

func NewHandlers(store *Store, encounterStore *encounter.Store, appointmentStore *appointment.Store,
	patientStore *patient.Store, practitionerStore *practitioner.Store, companyStore *company.Store,
	mastersStore *masters.Store, inventoryStore *inventory.Store, userStore *auth.Store,
	sm *scs.SessionManager, renderer *web.Renderer) *Handlers {
	return &Handlers{
		Store: store, EncounterStore: encounterStore, AppointmentStore: appointmentStore,
		PatientStore: patientStore, PractitionerStore: practitionerStore, CompanyStore: companyStore,
		MastersStore: mastersStore, InventoryStore: inventoryStore, UserStore: userStore,
		Sessions: sm, Renderer: renderer,
	}
}

func idParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

var (
	errQuickInvoiceBadPatient      = errors.New("invalid patient selection")
	errQuickInvoicePatientNotFound = errors.New("that patient could not be found")
	errQuickInvoiceNoPatient       = errors.New("please search for a patient, or enter a new patient's first name")
)

func (h *Handlers) redirectWithError(w http.ResponseWriter, r *http.Request, path, msg string) {
	http.Redirect(w, r, path+"?error="+url.QueryEscape(msg), http.StatusSeeOther)
}

// --- Index ---

type invoiceListData struct {
	Invoices []Invoice
	Status   string
	From, To string
}

func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := ListFilter{Status: q.Get("status")}
	if v := q.Get("from"); v != "" {
		if t, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
			filter.From = &t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
			t = t.Add(24 * time.Hour)
			filter.To = &t
		}
	}

	invoices, err := h.Store.List(r.Context(), filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.Render(w, r, "billing_index.html", invoiceListData{
		Invoices: invoices, Status: q.Get("status"), From: q.Get("from"), To: q.Get("to"),
	})
}

// --- Line item row view-model + shared parsing (encounter-triggered and quick invoice) ---

type lineItemRow struct {
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

func idAt(list []string, i int) *int64 {
	if i >= len(list) || list[i] == "" {
		return nil
	}
	id, err := strconv.ParseInt(list[i], 10, 64)
	if err != nil {
		return nil
	}
	return &id
}

func itemAt(list []string, i int) string {
	if i < len(list) {
		return list[i]
	}
	return ""
}

func floatAt(list []string, i int) float64 {
	f, err := strconv.ParseFloat(itemAt(list, i), 64)
	if err != nil {
		return 0
	}
	return f
}

// parseLineItems reads the posted kind[]/description[]/... parallel arrays
// (see billing_new.html / billing_quick.html), skipping any row with an
// empty description.
func parseLineItems(r *http.Request) []LineItemInput {
	kinds := r.PostForm["kind"]
	descriptions := r.PostForm["description"]
	hsnCodes := r.PostForm["hsn_sac_code"]
	quantities := r.PostForm["quantity"]
	unitPrices := r.PostForm["unit_price"]
	taxRates := r.PostForm["tax_rate"]
	labOrderIDs := r.PostForm["lab_order_id"]
	procedureOrderIDs := r.PostForm["procedure_order_id"]
	prescriptionItemIDs := r.PostForm["prescription_item_id"]

	var items []LineItemInput
	for i, desc := range descriptions {
		if desc == "" {
			continue
		}
		qty := floatAt(quantities, i)
		if qty == 0 {
			qty = 1
		}
		items = append(items, LineItemInput{
			Kind: itemAt(kinds, i), Description: desc, HSNSACCode: itemAt(hsnCodes, i),
			Quantity: qty, UnitPrice: floatAt(unitPrices, i), TaxRate: floatAt(taxRates, i),
			LabOrderID: idAt(labOrderIDs, i), ProcedureOrderID: idAt(procedureOrderIDs, i),
			PrescriptionItemID: idAt(prescriptionItemIDs, i),
		})
	}
	return items
}

// --- Encounter-triggered invoice ---

type invoiceFormData struct {
	Encounter *encounter.Encounter
	Patient   *patient.Patient
	Rows      []lineItemRow
	Error     string
}

func (h *Handlers) NewInvoiceForm(w http.ResponseWriter, r *http.Request) {
	encounterID, err := strconv.ParseInt(r.URL.Query().Get("encounter_id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()

	enc, err := h.EncounterStore.Get(ctx, encounterID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	pat, err := h.PatientStore.Get(ctx, enc.PatientID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var rows []lineItemRow

	// Consultation line: first-consultation fee unless the patient already
	// has an earlier encounter on record (this one included).
	clinic, err := h.CompanyStore.GetClinic(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	history, _ := h.EncounterStore.ListByPatient(ctx, enc.PatientID)
	consultDesc := "Consultation"
	fee := clinic.FirstConsultationFee
	if len(history) > 1 {
		consultDesc = "Follow-up consultation"
		fee = clinic.FollowUpFee
	}
	unitPrice := 0.0
	if fee != nil {
		unitPrice = *fee
	}
	rows = append(rows, lineItemRow{Kind: "consultation", Description: consultDesc, Quantity: 1, UnitPrice: unitPrice})

	items, err := h.EncounterStore.ListPrescriptionItems(ctx, encounterID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, it := range items {
		qty := 1.0
		if it.Quantity != nil {
			qty = *it.Quantity
		}
		price := 0.0
		if it.DrugID != nil {
			if d, err := h.InventoryStore.GetDrug(ctx, *it.DrugID); err == nil && d.Price != nil {
				price = *d.Price
			}
		}
		rows = append(rows, lineItemRow{
			Kind: "drug", Description: it.DrugName, Quantity: qty, UnitPrice: price,
			PrescriptionItemID: &it.ID,
		})
	}

	labOrders, err := h.EncounterStore.ListLabOrders(ctx, encounterID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, lo := range labOrders {
		price := 0.0
		if lo.LabTestTemplateID != nil {
			if t, err := h.MastersStore.GetLabTestTemplate(ctx, *lo.LabTestTemplateID); err == nil && t.Price != nil {
				price = *t.Price
			}
		}
		rows = append(rows, lineItemRow{
			Kind: "lab", Description: lo.TestName, Quantity: 1, UnitPrice: price, LabOrderID: &lo.ID,
		})
	}

	procOrders, err := h.EncounterStore.ListProcedureOrders(ctx, encounterID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, po := range procOrders {
		price := 0.0
		if po.ProcedureTemplateID != nil {
			if t, err := h.MastersStore.GetProcedureTemplate(ctx, *po.ProcedureTemplateID); err == nil && t.Price != nil {
				price = *t.Price
			}
		}
		rows = append(rows, lineItemRow{
			Kind: "procedure", Description: po.ProcedureName, Quantity: 1, UnitPrice: price, ProcedureOrderID: &po.ID,
		})
	}

	// One blank custom-line row.
	rows = append(rows, lineItemRow{Kind: "other", Quantity: 1})

	h.Renderer.Render(w, r, "billing_new.html", invoiceFormData{Encounter: enc, Patient: pat, Rows: rows})
}

func (h *Handlers) CreateInvoice(w http.ResponseWriter, r *http.Request) {
	encounterID, err := strconv.ParseInt(r.URL.Query().Get("encounter_id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()

	enc, err := h.EncounterStore.Get(ctx, encounterID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	items := parseLineItems(r)
	if len(items) == 0 {
		h.redirectWithError(w, r, "/billing/new?encounter_id="+strconv.FormatInt(encounterID, 10),
			"Please add at least one line item.")
		return
	}

	userID := auth.CurrentUserID(h.Sessions, r)
	id, err := h.Store.Create(ctx, CreateInvoiceInput{
		PatientID: enc.PatientID, PractitionerID: enc.PractitionerID,
		AppointmentID: enc.AppointmentID, EncounterID: &encounterID,
		DiscountAmount: floatAt(r.PostForm["discount_amount"], 0),
		Notes:          r.PostForm.Get("notes"), CreatedBy: &userID, Items: items,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/billing/invoices/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// --- Standalone quick invoice ---

type quickInvoiceFormData struct {
	PatientWidget patient.SearchWidgetConfig
	Practitioners []practitioner.Practitioner
	Rows          []lineItemRow
	Error         string
}

func (h *Handlers) NewQuickInvoiceForm(w http.ResponseWriter, r *http.Request) {
	practitioners, err := h.PractitionerStore.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := quickInvoiceFormData{
		PatientWidget: patient.SearchWidgetConfig{Mode: "embedded", SearchEndpoint: "/quick-prescription/patients"},
		Practitioners: practitioners,
		Rows:          []lineItemRow{{Kind: "other", Quantity: 1}, {Kind: "other", Quantity: 1}},
	}
	if msg := r.URL.Query().Get("error"); msg != "" {
		data.Error = msg
	}
	h.Renderer.Render(w, r, "billing_quick.html", data)
}

// resolveQuickInvoicePatient mirrors encounter.Handlers' quick-prescription
// patient resolution: use the posted patient_id, or create a new patient
// from the "new patient" panel fields.
func (h *Handlers) resolveQuickInvoicePatient(ctx context.Context, r *http.Request) (int64, error) {
	if idStr := r.PostForm.Get("patient_id"); idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return 0, errQuickInvoiceBadPatient
		}
		if _, err := h.PatientStore.Get(ctx, id); err != nil {
			return 0, errQuickInvoicePatientNotFound
		}
		return id, nil
	}
	firstName := r.PostForm.Get("first_name")
	if firstName == "" {
		return 0, errQuickInvoiceNoPatient
	}
	var age *int
	if v := r.PostForm.Get("age"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			age = &n
		}
	}
	return h.PatientStore.Create(ctx, patient.Input{
		FirstName: firstName, LastName: r.PostForm.Get("last_name"),
		Age: age, Sex: r.PostForm.Get("sex"), Phone: r.PostForm.Get("phone"),
	})
}

func (h *Handlers) CreateQuickInvoice(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx := r.Context()

	practitionerID, err := strconv.ParseInt(r.PostForm.Get("practitioner_id"), 10, 64)
	if err != nil {
		h.redirectWithError(w, r, "/billing/quick", "Please choose a practitioner.")
		return
	}
	if _, err := h.PractitionerStore.Get(ctx, practitionerID); err != nil {
		h.redirectWithError(w, r, "/billing/quick", "That practitioner could not be found.")
		return
	}

	patientID, err := h.resolveQuickInvoicePatient(ctx, r)
	if err != nil {
		h.redirectWithError(w, r, "/billing/quick", err.Error())
		return
	}

	items := parseLineItems(r)
	if len(items) == 0 {
		h.redirectWithError(w, r, "/billing/quick", "Please add at least one line item.")
		return
	}

	userID := auth.CurrentUserID(h.Sessions, r)
	id, err := h.Store.Create(ctx, CreateInvoiceInput{
		PatientID: patientID, PractitionerID: practitionerID,
		DiscountAmount: floatAt(r.PostForm["discount_amount"], 0),
		Notes:          r.PostForm.Get("notes"), CreatedBy: &userID, Items: items,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/billing/invoices/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// --- View / print / payments / cancel ---

type invoiceViewData struct {
	Invoice   *Invoice
	LineItems []LineItem
	Payments  []Payment
	Error     string
}

func (h *Handlers) ViewInvoice(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()

	inv, err := h.Store.Get(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	items, err := h.Store.ListLineItems(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	payments, err := h.Store.ListPayments(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := invoiceViewData{Invoice: inv, LineItems: items, Payments: payments}
	if msg := r.URL.Query().Get("error"); msg != "" {
		data.Error = msg
	}
	h.Renderer.Render(w, r, "billing_invoice.html", data)
}

type invoicePrintData struct {
	Invoice   *Invoice
	LineItems []LineItem
	Company   *company.Company
}

func (h *Handlers) PrintInvoice(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()

	inv, err := h.Store.Get(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	items, err := h.Store.ListLineItems(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c, err := h.CompanyStore.GetClinic(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.RenderStandalone(w, "invoice_print.html", invoicePrintData{Invoice: inv, LineItems: items, Company: c})
}

func (h *Handlers) RecordPayment(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	amount, err := strconv.ParseFloat(r.PostForm.Get("amount"), 64)
	if err != nil || amount <= 0 {
		h.redirectWithError(w, r, "/billing/invoices/"+strconv.FormatInt(id, 10), "Please enter a valid payment amount.")
		return
	}
	paidAt := time.Now()
	if v := r.PostForm.Get("paid_at"); v != "" {
		if t, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
			paidAt = t
		}
	}
	userID := auth.CurrentUserID(h.Sessions, r)
	err = h.Store.RecordPayment(r.Context(), id, PaymentInput{
		Amount: amount, Method: r.PostForm.Get("method"), ReferenceNo: r.PostForm.Get("reference_no"),
		Notes: r.PostForm.Get("notes"), PaidAt: paidAt, RecordedBy: &userID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/billing/invoices/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (h *Handlers) CancelInvoice(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.Store.Cancel(r.Context(), id); err != nil {
		h.redirectWithError(w, r, "/billing/invoices/"+strconv.FormatInt(id, 10), err.Error())
		return
	}
	http.Redirect(w, r, "/billing/invoices/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}
