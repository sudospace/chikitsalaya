package encounter

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"chikitsalaya/internal/auth"
	"chikitsalaya/internal/opd/appointment"
	"chikitsalaya/internal/opd/patient"
	"chikitsalaya/internal/opd/practitioner"
)

// Quick Prescription is a one-page shortcut around the normal
// appointment -> check-in -> consultation flow.
type quickRxFormData struct {
	Practitioners         []practitioner.Practitioner
	DosageOptionsJSON     template.JS
	DurationOptionsJSON   template.JS
	DefaultPractitionerID int64
	PatientWidget         patient.SearchWidgetConfig
	Error                 string
}

func (h *Handlers) QuickPrescriptionForm(w http.ResponseWriter, r *http.Request) {
	data, err := h.loadQuickRxForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if msg := r.URL.Query().Get("error"); msg != "" {
		data.Error = msg
	}
	h.Renderer.Render(w, r, "quick_prescription.html", *data)
}

func (h *Handlers) loadQuickRxForm(r *http.Request) (*quickRxFormData, error) {
	ctx := r.Context()

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

	var defaultPractitionerID int64
	if userID := auth.CurrentUserID(h.Sessions, r); userID != 0 {
		if u, err := h.UserStore.GetByID(ctx, userID); err == nil && u.PractitionerID != nil {
			defaultPractitionerID = *u.PractitionerID
		}
	}

	return &quickRxFormData{
		Practitioners:     practitioners,
		DosageOptionsJSON: lookupValuesJSON(dosages), DurationOptionsJSON: lookupValuesJSON(sortDurationOptions(durations)),
		DefaultPractitionerID: defaultPractitionerID,
		PatientWidget: patient.SearchWidgetConfig{
			Mode: "embedded", SearchEndpoint: "/quick-prescription/patients",
		},
	}, nil
}

// QuickPrescriptionSearchPatients is the htmx endpoint backing the
// existing-patient search box on the Quick Prescription form.
func (h *Handlers) QuickPrescriptionSearchPatients(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	patients, err := h.PatientStore.List(r.Context(), q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.RenderPartial(w, "patient_search_results", patients)
}

func (h *Handlers) redirectQuickRxError(w http.ResponseWriter, r *http.Request, msg string) {
	http.Redirect(w, r, "/quick-prescription?error="+url.QueryEscape(msg), http.StatusSeeOther)
}

// CreateQuickPrescription runs the whole quick-prescription flow in one
// request and redirects to the print view.
func (h *Handlers) CreateQuickPrescription(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	userID := auth.CurrentUserID(h.Sessions, r)

	practitionerID, err := strconv.ParseInt(r.PostForm.Get("practitioner_id"), 10, 64)
	if err != nil {
		h.redirectQuickRxError(w, r, "Please choose a prescribing doctor.")
		return
	}
	if _, err := h.PractitionerStore.Get(ctx, practitionerID); err != nil {
		h.redirectQuickRxError(w, r, "That doctor could not be found.")
		return
	}

	patientID, err := h.resolveQuickRxPatient(ctx, r)
	if err != nil {
		h.redirectQuickRxError(w, r, err.Error())
		return
	}

	visitType := "Consultation"
	if history, err := h.Store.ListByPatient(ctx, patientID); err == nil && len(history) > 0 {
		visitType = "Follow-up"
	}

	// Same doctor, same timestamp can collide on the usual no-overlap
	// check; nudge forward past the conflict instead of failing outright.
	const quickRxDuration = 5
	scheduledAt := time.Now()
	var apptID int64
	for attempt := 0; attempt < 20; attempt++ {
		apptID, err = h.AppointmentStore.Create(ctx, appointment.Input{
			PatientID: patientID, PractitionerID: practitionerID,
			ScheduledAt: scheduledAt, DurationMinutes: quickRxDuration, Status: "completed", Source: "staff",
			AppointmentType: visitType, ReasonForVisit: "Quick prescription", CreatedBy: &userID,
		})
		if err == nil {
			break
		}
		if !errors.Is(err, appointment.ErrSlotConflict) {
			h.redirectQuickRxError(w, r, "Couldn't create the appointment. Please try again.")
			return
		}
		scheduledAt = scheduledAt.Add(quickRxDuration * time.Minute)
	}
	if err != nil {
		h.redirectQuickRxError(w, r, "That doctor already has back-to-back visits logged just now. Please try again in a moment.")
		return
	}

	encID, err := h.Store.StartFromAppointment(ctx, apptID, patientID, practitionerID)
	if err != nil {
		h.redirectQuickRxError(w, r, "Couldn't start the consultation. Please try again.")
		return
	}

	var nextReview *time.Time
	if v := r.PostForm.Get("next_review_date"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			nextReview = &t
		}
	}
	notes := r.PostForm.Get("notes")
	plan := r.PostForm.Get("plan")
	if notes != "" || plan != "" || nextReview != nil {
		_ = h.Store.UpdateCore(ctx, encID, CoreInput{
			Assessment: notes, Plan: plan, NextReviewDate: nextReview,
		})
	}

	drugNames := r.PostForm["drug_name"]
	drugIDs := r.PostForm["drug_id"]
	dosageTexts := r.PostForm["dosage_text"]
	dosageIDs := r.PostForm["dosage_id"]
	durationTexts := r.PostForm["duration_text"]
	durationIDs := r.PostForm["duration_id"]
	quantities := r.PostForm["quantity"]
	remarks := r.PostForm["remark"]

	itemAt := func(list []string, i int) string {
		if i < len(list) {
			return list[i]
		}
		return ""
	}
	idAt := func(list []string, i int) *int64 {
		v := itemAt(list, i)
		if v == "" {
			return nil
		}
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil
		}
		return &id
	}

	for i, name := range drugNames {
		if name == "" {
			continue
		}
		_ = h.Store.AddPrescriptionItem(ctx, encID, PrescriptionItemInput{
			DrugID: idAt(drugIDs, i), DrugName: name,
			DosageID: idAt(dosageIDs, i), DosageText: itemAt(dosageTexts, i),
			DurationID: idAt(durationIDs, i), DurationText: itemAt(durationTexts, i),
			Quantity: parseOptionalFloat(itemAt(quantities, i)), Remark: itemAt(remarks, i),
		})
	}

	labTestNames := r.PostForm["test_name"]
	labTestIDs := r.PostForm["lab_test_template_id"]
	labComments := r.PostForm["comment"]
	for i, name := range labTestNames {
		if name == "" {
			continue
		}
		_ = h.Store.AddLabOrder(ctx, encID, patientID, LabOrderInput{
			LabTestTemplateID: idAt(labTestIDs, i), TestName: name, Comment: itemAt(labComments, i),
		})
	}

	procNames := r.PostForm["procedure_name"]
	procIDs := r.PostForm["procedure_template_id"]
	procComments := r.PostForm["comments"]
	for i, name := range procNames {
		if name == "" {
			continue
		}
		_ = h.Store.AddProcedureOrder(ctx, encID, patientID, ProcedureOrderInput{
			ProcedureTemplateID: idAt(procIDs, i), ProcedureName: name, Comments: itemAt(procComments, i),
		})
	}

	if err := h.Store.Complete(ctx, encID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/encounters/"+strconv.FormatInt(encID, 10)+"/print", http.StatusSeeOther)
}

// resolveQuickRxPatient uses the posted patient_id if present, otherwise
// creates a new patient from the "new patient" panel fields.
func (h *Handlers) resolveQuickRxPatient(ctx context.Context, r *http.Request) (int64, error) {
	if idStr := r.PostForm.Get("patient_id"); idStr != "" {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			return 0, errors.New("invalid patient selection")
		}
		if _, err := h.PatientStore.Get(ctx, id); err != nil {
			return 0, errors.New("that patient could not be found")
		}
		return id, nil
	}

	firstName := r.PostForm.Get("first_name")
	if firstName == "" {
		return 0, errors.New("please search for a patient, or enter a new patient's first name")
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
