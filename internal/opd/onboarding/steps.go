package onboarding

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"chikitsalaya/internal/auth"
	"chikitsalaya/internal/opd/inventory"
	"chikitsalaya/internal/opd/masters"
	"chikitsalaya/internal/opd/practitioner"
	"chikitsalaya/internal/opd/schedule"
)

// --- Step 1: practitioners (+ optional schedule blocks) ---

type practitionerRow struct {
	Practitioner practitioner.Practitioner
	Schedules    []schedule.Schedule
}

type practitionersStepData struct {
	Practitioners []practitionerRow
	Error         string
}

func (h *Handlers) PractitionersStep(w http.ResponseWriter, r *http.Request) {
	practitioners, err := h.PractitionerStore.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rows := make([]practitionerRow, 0, len(practitioners))
	for _, p := range practitioners {
		schedules, err := h.ScheduleStore.ListByPractitioner(r.Context(), p.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		rows = append(rows, practitionerRow{Practitioner: p, Schedules: schedules})
	}
	h.Renderer.Render(w, r, "onboarding_practitioners.html", practitionersStepData{Practitioners: rows})
}

func (h *Handlers) CreatePractitionerStep(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	fullName := r.PostForm.Get("full_name")
	if fullName == "" {
		h.renderPractitionersStep(w, r, "Please give the practitioner a name.")
		return
	}
	_, err := h.PractitionerStore.Create(r.Context(), practitioner.Input{
		FullName:       fullName,
		Qualification:  r.PostForm.Get("qualification"),
		Specialization: r.PostForm.Get("specialization"),
		Phone:          r.PostForm.Get("phone"),
		Email:          r.PostForm.Get("email"),
		IsActive:       true,
	})
	if err != nil {
		h.renderPractitionersStep(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/onboarding/practitioners", http.StatusSeeOther)
}

func (h *Handlers) renderPractitionersStep(w http.ResponseWriter, r *http.Request, errMsg string) {
	practitioners, _ := h.PractitionerStore.List(r.Context())
	rows := make([]practitionerRow, 0, len(practitioners))
	for _, p := range practitioners {
		schedules, _ := h.ScheduleStore.ListByPractitioner(r.Context(), p.ID)
		rows = append(rows, practitionerRow{Practitioner: p, Schedules: schedules})
	}
	h.Renderer.Render(w, r, "onboarding_practitioners.html", practitionersStepData{Practitioners: rows, Error: errMsg})
}

// CreatePractitionerSchedule adds a schedule block to an existing practitioner.
func (h *Handlers) CreatePractitionerSchedule(w http.ResponseWriter, r *http.Request) {
	practitionerID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if _, err := h.PractitionerStore.Get(r.Context(), practitionerID); err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dayOfWeek, err1 := strconv.Atoi(r.PostForm.Get("day_of_week"))
	slotDuration, err2 := strconv.Atoi(r.PostForm.Get("slot_duration_minutes"))
	if err1 != nil || err2 != nil || slotDuration <= 0 {
		h.renderPractitionersStep(w, r, "Please provide a valid day, time range, and slot length.")
		return
	}
	_, err = h.ScheduleStore.Create(r.Context(), schedule.Input{
		PractitionerID:      practitionerID,
		DayOfWeek:           dayOfWeek,
		StartTime:           r.PostForm.Get("start_time"),
		EndTime:             r.PostForm.Get("end_time"),
		SlotDurationMinutes: slotDuration,
		IsActive:            true,
	})
	if err != nil {
		h.renderPractitionersStep(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/onboarding/practitioners", http.StatusSeeOther)
}

// --- Step 2: drugs ---

type drugsStepData struct {
	Drugs []inventory.Drug
	Error string
}

func (h *Handlers) DrugsStep(w http.ResponseWriter, r *http.Request) {
	drugs, err := h.InventoryStore.ListDrugs(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.Render(w, r, "onboarding_drugs.html", drugsStepData{Drugs: drugs})
}

func (h *Handlers) CreateDrugStep(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := r.PostForm.Get("name")
	if name == "" {
		h.renderDrugsStep(w, r, "Please give the drug a name.")
		return
	}
	var opening float64
	if v := r.PostForm.Get("opening_stock"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			h.renderDrugsStep(w, r, "Invalid opening stock.")
			return
		}
		opening = f
	}
	_, err := h.InventoryStore.CreateDrug(r.Context(), inventory.DrugInput{
		Name: name, GenericName: r.PostForm.Get("generic_name"),
		Form: r.PostForm.Get("form"), Strength: r.PostForm.Get("strength"), Unit: r.PostForm.Get("unit"),
		OpeningStock: opening, IsActive: true,
	}, auth.CurrentUserID(h.Sessions, r))
	if err != nil {
		h.renderDrugsStep(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/onboarding/drugs", http.StatusSeeOther)
}

func (h *Handlers) renderDrugsStep(w http.ResponseWriter, r *http.Request, errMsg string) {
	drugs, _ := h.InventoryStore.ListDrugs(r.Context())
	h.Renderer.Render(w, r, "onboarding_drugs.html", drugsStepData{Drugs: drugs, Error: errMsg})
}

// --- Step 3: lab tests ---

type labTestsStepData struct {
	Templates []masters.LabTestTemplate
	Error     string
}

func (h *Handlers) LabTestsStep(w http.ResponseWriter, r *http.Request) {
	templates, err := h.MastersStore.ListLabTestTemplates(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.Render(w, r, "onboarding_lab_tests.html", labTestsStepData{Templates: templates})
}

func (h *Handlers) CreateLabTestStep(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := r.PostForm.Get("name")
	if name == "" {
		h.renderLabTestsStep(w, r, "Please give the lab test a name.")
		return
	}
	_, err := h.MastersStore.CreateLabTestTemplate(r.Context(), masters.LabTestTemplateInput{
		Name: name, Department: r.PostForm.Get("department"),
		SampleType: r.PostForm.Get("sample_type"), IsActive: true,
	})
	if err != nil {
		h.renderLabTestsStep(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/onboarding/lab-tests", http.StatusSeeOther)
}

func (h *Handlers) renderLabTestsStep(w http.ResponseWriter, r *http.Request, errMsg string) {
	templates, _ := h.MastersStore.ListLabTestTemplates(r.Context())
	h.Renderer.Render(w, r, "onboarding_lab_tests.html", labTestsStepData{Templates: templates, Error: errMsg})
}

// --- Step 4: procedures ---

type proceduresStepData struct {
	Templates []masters.ProcedureTemplate
	Error     string
}

func (h *Handlers) ProceduresStep(w http.ResponseWriter, r *http.Request) {
	templates, err := h.MastersStore.ListProcedureTemplates(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.Render(w, r, "onboarding_procedures.html", proceduresStepData{Templates: templates})
}

func (h *Handlers) CreateProcedureStep(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := r.PostForm.Get("name")
	if name == "" {
		h.renderProceduresStep(w, r, "Please give the procedure a name.")
		return
	}
	_, err := h.MastersStore.CreateProcedureTemplate(r.Context(), masters.ProcedureTemplateInput{
		Name: name, Department: r.PostForm.Get("department"),
		Description: r.PostForm.Get("description"), IsActive: true,
	})
	if err != nil {
		h.renderProceduresStep(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/onboarding/procedures", http.StatusSeeOther)
}

func (h *Handlers) renderProceduresStep(w http.ResponseWriter, r *http.Request, errMsg string) {
	templates, _ := h.MastersStore.ListProcedureTemplates(r.Context())
	h.Renderer.Render(w, r, "onboarding_procedures.html", proceduresStepData{Templates: templates, Error: errMsg})
}

// Finish marks the admin onboarded and sends them into the app.
func (h *Handlers) Finish(w http.ResponseWriter, r *http.Request) {
	if err := h.UserStore.MarkOnboarded(r.Context(), auth.CurrentUserID(h.Sessions, r)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
