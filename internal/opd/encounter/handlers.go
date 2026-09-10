package encounter

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"

	"chikitsalaya/internal/auth"
	"chikitsalaya/internal/opd/appointment"
	"chikitsalaya/internal/opd/company"
	"chikitsalaya/internal/opd/inventory"
	"chikitsalaya/internal/opd/masters"
	"chikitsalaya/internal/opd/patient"
	"chikitsalaya/internal/opd/practitioner"
	"chikitsalaya/internal/web"
)

type Handlers struct {
	Store             *Store
	AppointmentStore  *appointment.Store
	PractitionerStore *practitioner.Store
	InventoryStore    *inventory.Store
	MastersStore      *masters.Store
	CompanyStore      *company.Store
	PatientStore      *patient.Store
	UserStore         *auth.Store
	Sessions          *scs.SessionManager
	Renderer          *web.Renderer
}

func NewHandlers(store *Store, appointmentStore *appointment.Store, practitionerStore *practitioner.Store,
	inventoryStore *inventory.Store, mastersStore *masters.Store, companyStore *company.Store, patientStore *patient.Store,
	userStore *auth.Store, sm *scs.SessionManager, renderer *web.Renderer) *Handlers {
	return &Handlers{
		Store: store, AppointmentStore: appointmentStore, PractitionerStore: practitionerStore,
		InventoryStore: inventoryStore, MastersStore: mastersStore, CompanyStore: companyStore, PatientStore: patientStore,
		UserStore: userStore, Sessions: sm, Renderer: renderer,
	}
}

// StartConsultation creates (or resumes) the encounter for a checked-in
// appointment and sends the doctor straight to the workspace.
func (h *Handlers) StartConsultation(w http.ResponseWriter, r *http.Request) {
	appointmentID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	appt, err := h.AppointmentStore.Get(r.Context(), appointmentID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	id, err := h.Store.StartFromAppointment(r.Context(), appointmentID, appt.PatientID, appt.PractitionerID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/encounters/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

type workspaceData struct {
	Encounter           *Encounter
	Vitals              *VitalSigns
	Diagnoses           []Diagnosis
	PrescriptionItems   []PrescriptionItem
	LabOrders           []LabOrder
	ProcedureOrders     []ProcedureOrder
	Practitioners       []practitioner.Practitioner
	DosageOptionsJSON   template.JS
	DurationOptionsJSON template.JS
	Error               string
}

func (h *Handlers) Workspace(w http.ResponseWriter, r *http.Request) {
	data, err := h.loadWorkspace(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.Renderer.Render(w, r, "encounter_workspace.html", *data)
}

func (h *Handlers) loadWorkspace(r *http.Request) (*workspaceData, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		return nil, err
	}
	ctx := r.Context()

	enc, err := h.Store.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	vitals, _ := h.Store.GetVitals(ctx, id) // no rows yet is fine, ignore error
	diagnoses, err := h.Store.ListDiagnoses(ctx, id)
	if err != nil {
		return nil, err
	}
	items, err := h.Store.ListPrescriptionItems(ctx, id)
	if err != nil {
		return nil, err
	}
	labOrders, err := h.Store.ListLabOrders(ctx, id)
	if err != nil {
		return nil, err
	}
	procOrders, err := h.Store.ListProcedureOrders(ctx, id)
	if err != nil {
		return nil, err
	}
	practitioners, err := h.PractitionerStore.List(ctx)
	if err != nil {
		return nil, err
	}
	dosages, err := h.MastersStore.ListLookupByCategory(ctx, "dosage")
	if err != nil {
		return nil, err
	}
	durations, err := h.MastersStore.ListLookupByCategory(ctx, "duration")
	if err != nil {
		return nil, err
	}
	return &workspaceData{
		Encounter: enc, Vitals: vitals, Diagnoses: diagnoses, PrescriptionItems: items,
		LabOrders: labOrders, ProcedureOrders: procOrders, Practitioners: practitioners,
		DosageOptionsJSON: lookupValuesJSON(dosages), DurationOptionsJSON: lookupValuesJSON(sortDurationOptions(durations)),
	}, nil
}

func (h *Handlers) renderWorkspaceError(w http.ResponseWriter, r *http.Request, msg string) {
	data, err := h.loadWorkspace(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	data.Error = msg
	h.Renderer.Render(w, r, "encounter_workspace.html", *data)
}

func idParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

// --- Core fields ---

func (h *Handlers) UpdateCore(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var nextReview *time.Time
	if v := r.PostForm.Get("next_review_date"); v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err == nil {
			nextReview = &t
		}
	}
	var reviewBy *int64
	if v := r.PostForm.Get("review_by_practitioner_id"); v != "" {
		if pid, err := strconv.ParseInt(v, 10, 64); err == nil {
			reviewBy = &pid
		}
	}

	err = h.Store.UpdateCore(r.Context(), id, CoreInput{
		ChiefComplaint:          r.PostForm.Get("chief_complaint"),
		HistoryOfPresentIllness: r.PostForm.Get("history_of_present_illness"),
		ExaminationNotes:        r.PostForm.Get("examination_notes"),
		Assessment:              r.PostForm.Get("assessment"),
		Plan:                    r.PostForm.Get("plan"),
		NextReviewDate:          nextReview,
		ReviewByPractitionerID:  reviewBy,
	})
	if err != nil {
		h.renderWorkspaceError(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/encounters/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// --- Vitals ---

func (h *Handlers) UpsertVitalsHandler(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	in := VitalSignsInput{
		HeightCM:     parseOptionalFloat(r.PostForm.Get("height_cm")),
		WeightKG:     parseOptionalFloat(r.PostForm.Get("weight_kg")),
		TemperatureC: parseOptionalFloat(r.PostForm.Get("temperature_c")),
		PulseBPM:     parseOptionalInt(r.PostForm.Get("pulse_bpm")),
		RespRate:     parseOptionalInt(r.PostForm.Get("resp_rate")),
		BPSystolic:   parseOptionalInt(r.PostForm.Get("bp_systolic")),
		BPDiastolic:  parseOptionalInt(r.PostForm.Get("bp_diastolic")),
		SpO2:         parseOptionalInt(r.PostForm.Get("spo2")),
	}
	if err := h.Store.UpsertVitals(r.Context(), id, in); err != nil {
		h.renderWorkspaceError(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/encounters/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func parseOptionalFloat(v string) *float64 {
	if v == "" {
		return nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return nil
	}
	return &f
}

func parseOptionalInt(v string) *int {
	if v == "" {
		return nil
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return nil
	}
	return &i
}

// --- Diagnoses ---

type icd10SearchData struct {
	Results []ICD10Code
}

func (h *Handlers) SearchICD10Handler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		h.Renderer.RenderPartial(w, "icd10_results", icd10SearchData{})
		return
	}
	results, err := h.Store.SearchICD10(r.Context(), q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.RenderPartial(w, "icd10_results", icd10SearchData{Results: results})
}

func (h *Handlers) AddDiagnosisHandler(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var code *string
	if v := r.PostForm.Get("icd10_code"); v != "" {
		code = &v
	}
	err = h.Store.AddDiagnosis(r.Context(), id, DiagnosisInput{
		ICD10Code: code, Description: r.PostForm.Get("description"),
		DiagnosisType: r.PostForm.Get("diagnosis_type"),
	})
	if err != nil {
		h.renderWorkspaceError(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/encounters/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (h *Handlers) DeleteDiagnosisHandler(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	diagID, err := strconv.ParseInt(chi.URLParam(r, "diagID"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.Store.DeleteDiagnosis(r.Context(), diagID, id); err != nil {
		h.renderWorkspaceError(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/encounters/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// --- Prescription items ---

func (h *Handlers) AddPrescriptionItemHandler(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	in := PrescriptionItemInput{
		DrugName:     r.PostForm.Get("drug_name"),
		DosageText:   r.PostForm.Get("dosage_text"),
		DurationText: r.PostForm.Get("duration_text"),
		Quantity:     parseOptionalFloat(r.PostForm.Get("quantity")),
		Remark:       r.PostForm.Get("remark"),
	}
	if v := r.PostForm.Get("drug_id"); v != "" {
		if did, err := strconv.ParseInt(v, 10, 64); err == nil {
			in.DrugID = &did
			if d, err := h.InventoryStore.GetDrug(r.Context(), did); err == nil {
				in.DrugName = d.Name
			}
		}
	}
	if in.DrugName == "" {
		h.renderWorkspaceError(w, r, "Please choose a drug from the list or type a name.")
		return
	}
	if v := r.PostForm.Get("dosage_id"); v != "" {
		if did, err := strconv.ParseInt(v, 10, 64); err == nil {
			in.DosageID = &did
			if lv, err := h.MastersStore.GetLookupValue(r.Context(), did); err == nil {
				in.DosageText = lv.Value
			}
		}
	}
	if v := r.PostForm.Get("duration_id"); v != "" {
		if did, err := strconv.ParseInt(v, 10, 64); err == nil {
			in.DurationID = &did
			if lv, err := h.MastersStore.GetLookupValue(r.Context(), did); err == nil {
				in.DurationText = lv.Value
			}
		}
	}

	if err := h.Store.AddPrescriptionItem(r.Context(), id, in); err != nil {
		h.renderWorkspaceError(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/encounters/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (h *Handlers) DeletePrescriptionItemHandler(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	itemID, err := strconv.ParseInt(chi.URLParam(r, "itemID"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.Store.DeletePrescriptionItem(r.Context(), itemID, id); err != nil {
		h.renderWorkspaceError(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/encounters/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// --- Lab orders ---

func (h *Handlers) AddLabOrderHandler(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	enc, err := h.Store.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	in := LabOrderInput{TestName: r.PostForm.Get("test_name"), Comment: r.PostForm.Get("comment")}
	if v := r.PostForm.Get("lab_test_template_id"); v != "" {
		if tid, err := strconv.ParseInt(v, 10, 64); err == nil {
			in.LabTestTemplateID = &tid
			if t, err := h.MastersStore.GetLabTestTemplate(r.Context(), tid); err == nil {
				in.TestName = t.Name
			}
		}
	}
	if in.TestName == "" {
		h.renderWorkspaceError(w, r, "Please choose a lab test from the list or type a name.")
		return
	}

	if err := h.Store.AddLabOrder(r.Context(), id, enc.PatientID, in); err != nil {
		h.renderWorkspaceError(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/encounters/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (h *Handlers) RecordLabResultHandler(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	labID, err := strconv.ParseInt(chi.URLParam(r, "labID"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	err = h.Store.RecordLabResult(r.Context(), labID, id, LabResultInput{
		ResultValue: r.PostForm.Get("result_value"), ResultUnit: r.PostForm.Get("result_unit"),
		ReferenceRange: r.PostForm.Get("reference_range"), ResultNotes: r.PostForm.Get("result_notes"),
	})
	if err != nil {
		h.renderWorkspaceError(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/encounters/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (h *Handlers) DeleteLabOrderHandler(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	labID, err := strconv.ParseInt(chi.URLParam(r, "labID"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.Store.DeleteLabOrder(r.Context(), labID, id); err != nil {
		h.renderWorkspaceError(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/encounters/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// --- Procedure orders ---

func (h *Handlers) AddProcedureOrderHandler(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	enc, err := h.Store.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	in := ProcedureOrderInput{ProcedureName: r.PostForm.Get("procedure_name"), Comments: r.PostForm.Get("comments")}
	if v := r.PostForm.Get("procedure_template_id"); v != "" {
		if tid, err := strconv.ParseInt(v, 10, 64); err == nil {
			in.ProcedureTemplateID = &tid
			if t, err := h.MastersStore.GetProcedureTemplate(r.Context(), tid); err == nil {
				in.ProcedureName = t.Name
			}
		}
	}
	if in.ProcedureName == "" {
		h.renderWorkspaceError(w, r, "Please choose a procedure from the list or type a name.")
		return
	}

	if err := h.Store.AddProcedureOrder(r.Context(), id, enc.PatientID, in); err != nil {
		h.renderWorkspaceError(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/encounters/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (h *Handlers) DeleteProcedureOrderHandler(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	procID, err := strconv.ParseInt(chi.URLParam(r, "procID"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.Store.DeleteProcedureOrder(r.Context(), procID, id); err != nil {
		h.renderWorkspaceError(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/encounters/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// --- Complete ---

func (h *Handlers) CompleteHandler(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	enc, err := h.Store.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := h.Store.DispenseStock(r.Context(), id, auth.CurrentUserID(h.Sessions, r)); err != nil {
		h.renderWorkspaceError(w, r, err.Error())
		return
	}
	if err := h.Store.Complete(r.Context(), id); err != nil {
		h.renderWorkspaceError(w, r, err.Error())
		return
	}
	if enc.AppointmentID != nil {
		_ = h.AppointmentStore.UpdateStatus(r.Context(), *enc.AppointmentID, "completed")
	}

	http.Redirect(w, r, "/encounters/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// ReopenHandler puts a completed encounter back to in_progress so it can
// be corrected; re-completing safely re-runs DispenseStock.
func (h *Handlers) ReopenHandler(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.Store.Reopen(r.Context(), id); err != nil {
		h.renderWorkspaceError(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/encounters/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// --- Quick-add ---

func (h *Handlers) QuickAddDrugHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.PostForm.Get("drug_name"))
	if name != "" {
		// Case/whitespace-insensitive dedup: reuse an existing match instead of inserting.
		if _, err := h.InventoryStore.FindDrugByName(r.Context(), name); err != nil {
			if _, err := h.InventoryStore.CreateDrug(r.Context(), inventory.DrugInput{
				Name: name, IsActive: true,
			}, auth.CurrentUserID(h.Sessions, r)); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
	hits, err := h.searchDrugHits(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.RenderPartial(w, "catalog_search_results", catalogSearchData{
		Query: name, Kind: "drug", QuickAddEndpoint: "/catalog/quick-add-drug", QuickAddField: "drug_name", Hits: hits,
	})
}

func (h *Handlers) QuickAddLabTestHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.PostForm.Get("test_name"))
	if name != "" {
		if _, err := h.MastersStore.FindLabTestTemplateByName(r.Context(), name); err != nil {
			if _, err := h.MastersStore.CreateLabTestTemplate(r.Context(), masters.LabTestTemplateInput{
				Name: name, IsActive: true,
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
	hits, err := h.searchLabTestHits(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.RenderPartial(w, "catalog_search_results", catalogSearchData{
		Query: name, Kind: "lab test", QuickAddEndpoint: "/catalog/quick-add-lab-test", QuickAddField: "test_name", Hits: hits,
	})
}

func (h *Handlers) QuickAddProcedureHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.PostForm.Get("procedure_name"))
	if name != "" {
		// Case/whitespace-insensitive dedup: reuse an existing match instead of inserting.
		if _, err := h.MastersStore.FindProcedureTemplateByName(r.Context(), name); err != nil {
			if _, err := h.MastersStore.CreateProcedureTemplate(r.Context(), masters.ProcedureTemplateInput{
				Name: name, IsActive: true,
			}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}
	hits, err := h.searchProcedureHits(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.RenderPartial(w, "catalog_search_results", catalogSearchData{
		Query: name, Kind: "procedure", QuickAddEndpoint: "/catalog/quick-add-procedure", QuickAddField: "procedure_name", Hits: hits,
	})
}

// QuickAddLookupValueHandler dedupes-and-creates a dosage/duration picklist
// value, returning the refreshed category options as JSON (not an HTML
// partial like the other three quick-add handlers) since dosage/duration
// are cached client-side as input._options, not backed by an on-page
// <datalist>.
func (h *Handlers) QuickAddLookupValueHandler(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	category := r.PostForm.Get("category")
	if category != "dosage" && category != "duration" {
		http.Error(w, "invalid category", http.StatusBadRequest)
		return
	}
	value := strings.TrimSpace(r.PostForm.Get("value"))
	if value == "" {
		http.Error(w, "value is required", http.StatusBadRequest)
		return
	}
	if _, err := h.MastersStore.FindLookupValueByName(r.Context(), category, value); err != nil {
		if _, err := h.MastersStore.CreateLookupValue(r.Context(), masters.LookupValueInput{
			Category: category, Value: value,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	options, err := h.MastersStore.ListLookupByCategory(r.Context(), category)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if category == "duration" {
		options = sortDurationOptions(options)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(toLookupOptionJSON(options)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// --- Print ---

type printData struct {
	Encounter         *Encounter
	Patient           *patient.Patient
	Company           *company.Company
	LetterheadHTML    template.HTML
	Vitals            *VitalSigns
	Diagnoses         []Diagnosis
	PrescriptionItems []PrescriptionItem
	LabOrders         []LabOrder
	ProcedureOrders   []ProcedureOrder
}

func (h *Handlers) PrintHandler(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()

	enc, err := h.Store.Get(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	pat, err := h.PatientStore.Get(ctx, enc.PatientID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c, err := h.CompanyStore.GetClinic(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	vitals, _ := h.Store.GetVitals(ctx, id) // no rows yet is fine, ignore error
	diagnoses, err := h.Store.ListDiagnoses(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	items, err := h.Store.ListPrescriptionItems(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	labOrders, err := h.Store.ListLabOrders(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	procOrders, err := h.Store.ListProcedureOrders(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// LetterheadHTML is trusted admin input, meant to be rendered as markup.
	h.Renderer.RenderStandalone(w, "encounter_print.html", printData{
		Encounter: enc, Patient: pat, Company: c, LetterheadHTML: template.HTML(c.LetterheadHTML),
		Vitals: vitals, Diagnoses: diagnoses, PrescriptionItems: items, LabOrders: labOrders, ProcedureOrders: procOrders,
	})
}

// --- Patient history ---

// historyEntry is one visit's worth of everything captured during it, so
// the timeline groups by encounter rather than interleaving every table.
type historyEntry struct {
	Encounter         Encounter
	Vitals            *VitalSigns
	Diagnoses         []Diagnosis
	PrescriptionItems []PrescriptionItem
	LabOrders         []LabOrder
	ProcedureOrders   []ProcedureOrder
}

type historyData struct {
	Patient *patient.Patient
	Entries []historyEntry
}

// PatientHistory shows every encounter a patient has ever had across every
// company — patients are a shared directory, not scoped to one clinic.
func (h *Handlers) PatientHistory(w http.ResponseWriter, r *http.Request) {
	patientID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()

	p, err := h.PatientStore.Get(ctx, patientID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	encounters, err := h.Store.ListByPatient(ctx, patientID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	entries := make([]historyEntry, 0, len(encounters))
	for _, enc := range encounters {
		vitals, _ := h.Store.GetVitals(ctx, enc.ID) // no rows yet is fine, ignore error
		diagnoses, err := h.Store.ListDiagnoses(ctx, enc.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		items, err := h.Store.ListPrescriptionItems(ctx, enc.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		labOrders, err := h.Store.ListLabOrders(ctx, enc.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		procOrders, err := h.Store.ListProcedureOrders(ctx, enc.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		entries = append(entries, historyEntry{
			Encounter: enc, Vitals: vitals, Diagnoses: diagnoses,
			PrescriptionItems: items, LabOrders: labOrders, ProcedureOrders: procOrders,
		})
	}

	h.Renderer.Render(w, r, "patient_history.html", historyData{Patient: p, Entries: entries})
}
