package appointment

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"

	"chikitsalaya/internal/auth"
	"chikitsalaya/internal/opd/company"
	"chikitsalaya/internal/opd/patient"
	"chikitsalaya/internal/opd/practitioner"
	"chikitsalaya/internal/opd/schedule"
	"chikitsalaya/internal/web"
)

const dateLayout = "2006-01-02"
const slotValueLayout = "2006-01-02T15:04"

type Handlers struct {
	Store             *Store
	PatientStore      *patient.Store
	PractitionerStore *practitioner.Store
	ScheduleStore     *schedule.Store
	CompanyStore      *company.Store
	Sessions          *scs.SessionManager
	Renderer          *web.Renderer
}

func NewHandlers(store *Store, patientStore *patient.Store, practitionerStore *practitioner.Store,
	scheduleStore *schedule.Store, companyStore *company.Store, sm *scs.SessionManager, renderer *web.Renderer) *Handlers {
	return &Handlers{
		Store: store, PatientStore: patientStore, PractitionerStore: practitionerStore,
		ScheduleStore: scheduleStore, CompanyStore: companyStore, Sessions: sm, Renderer: renderer,
	}
}

// --- Day view ---

type indexData struct {
	Practitioners  []practitioner.Practitioner
	PractitionerID int64
	Date           string
	Practitioner   *practitioner.Practitioner
	Appointments   []Appointment
	Company        *company.Company
}

func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	practitioners, err := h.PractitionerStore.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c, err := h.CompanyStore.GetClinic(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := indexData{Practitioners: practitioners, Date: dateParam(r), Company: c}

	if pidStr := r.URL.Query().Get("practitioner_id"); pidStr != "" {
		pid, err := strconv.ParseInt(pidStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid practitioner_id", http.StatusBadRequest)
			return
		}
		p, err := h.PractitionerStore.Get(r.Context(), pid)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		date, err := time.ParseInLocation(dateLayout, data.Date, time.Local)
		if err != nil {
			http.Error(w, "invalid date", http.StatusBadRequest)
			return
		}
		appts, err := h.Store.ListByPractitionerAndDate(r.Context(), pid, date)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		data.PractitionerID = pid
		data.Practitioner = p
		data.Appointments = appts
	}

	h.Renderer.Render(w, r, "appointments_index.html", data)
}

func dateParam(r *http.Request) string {
	if v := r.URL.Query().Get("date"); v != "" {
		return v
	}
	return time.Now().Format(dateLayout)
}

// --- Booking flow: patient search -> practitioner/date -> slot grid -> book ---

type bookingSearchData struct {
	Query         string
	Error         string
	PatientWidget patient.SearchWidgetConfig
}

type bookingData struct {
	Patient        *patient.Patient
	Practitioners  []practitioner.Practitioner
	PractitionerID int64
	Date           string
	Error          string
	Company        *company.Company
}

func (h *Handlers) New(w http.ResponseWriter, r *http.Request) {
	patientIDStr := r.URL.Query().Get("patient_id")
	if patientIDStr == "" {
		data := bookingSearchData{
			Query: r.URL.Query().Get("q"),
			PatientWidget: patient.SearchWidgetConfig{
				Mode: "standalone", SearchEndpoint: "/appointments/new/search",
				NavigateTemplate: "/appointments/new?patient_id={id}", ActionURL: "/appointments/new/patients",
			},
		}
		if msg := r.URL.Query().Get("error"); msg != "" {
			data.Error = msg
		}
		h.Renderer.Render(w, r, "appointment_booking_search.html", data)
		return
	}

	patientID, err := strconv.ParseInt(patientIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid patient_id", http.StatusBadRequest)
		return
	}
	pat, err := h.PatientStore.Get(r.Context(), patientID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	practitioners, err := h.PractitionerStore.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c, err := h.CompanyStore.GetClinic(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := bookingData{Patient: pat, Practitioners: practitioners, Date: dateParam(r), Company: c}
	if msg := r.URL.Query().Get("error"); msg != "" {
		data.Error = msg
	}
	if pidStr := r.URL.Query().Get("practitioner_id"); pidStr != "" {
		if pid, err := strconv.ParseInt(pidStr, 10, 64); err == nil {
			data.PractitionerID = pid
		}
	}
	h.Renderer.Render(w, r, "appointment_booking.html", data)
}

// SearchPatients backs live patient search inside the booking flow.
func (h *Handlers) SearchPatients(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	patients, err := h.PatientStore.List(r.Context(), q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.RenderPartial(w, "patient_search_results", patients)
}

// CreatePatientForBooking creates a patient inline from the booking search
// screen's "add as a new patient" panel, then continues straight to step 2
// (practitioner/date/slot) instead of the old dead-end link to /patients/new.
func (h *Handlers) CreatePatientForBooking(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	firstName := r.PostForm.Get("first_name")
	if firstName == "" {
		http.Redirect(w, r, "/appointments/new?error="+url.QueryEscape("Please enter a first name."), http.StatusSeeOther)
		return
	}
	var age *int
	if v := r.PostForm.Get("age"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			age = &n
		}
	}
	id, err := h.PatientStore.Create(r.Context(), patient.Input{
		FirstName: firstName, LastName: r.PostForm.Get("last_name"),
		Age: age, Sex: r.PostForm.Get("sex"), Phone: r.PostForm.Get("phone"),
	})
	if err != nil {
		http.Redirect(w, r, "/appointments/new?error="+url.QueryEscape("Couldn't create the patient. Please try again."), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/appointments/new?patient_id="+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

type slotsData struct {
	PatientID      int64
	PractitionerID int64
	Date           string
	Slots          []Slot
}

// Slots renders the available-slot grid for a practitioner + date.
func (h *Handlers) Slots(w http.ResponseWriter, r *http.Request) {
	patientID, _ := strconv.ParseInt(r.URL.Query().Get("patient_id"), 10, 64)
	practitionerID, err := strconv.ParseInt(r.URL.Query().Get("practitioner_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid practitioner_id", http.StatusBadRequest)
		return
	}
	dateStr := r.URL.Query().Get("date")
	date, err := time.ParseInLocation(dateLayout, dateStr, time.Local)
	if err != nil {
		http.Error(w, "invalid date", http.StatusBadRequest)
		return
	}

	available, err := h.availableSlots(r, practitionerID, date)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.Renderer.RenderPartial(w, "appointment_slots", slotsData{
		PatientID: patientID, PractitionerID: practitionerID, Date: dateStr, Slots: available,
	})
}

func (h *Handlers) availableSlots(r *http.Request, practitionerID int64, date time.Time) ([]Slot, error) {
	schedules, err := h.ScheduleStore.ListByPractitioner(r.Context(), practitionerID)
	if err != nil {
		return nil, err
	}
	generated, err := GenerateDaySlots(schedules, date)
	if err != nil {
		return nil, err
	}
	booked, err := h.Store.BookedRanges(r.Context(), practitionerID, date)
	if err != nil {
		return nil, err
	}
	return AvailableSlots(generated, booked), nil
}

// --- Create ---

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	patientID, err1 := strconv.ParseInt(r.PostForm.Get("patient_id"), 10, 64)
	practitionerID, err2 := strconv.ParseInt(r.PostForm.Get("practitioner_id"), 10, 64)
	scheduledAt, err3 := time.ParseInLocation(slotValueLayout, r.PostForm.Get("scheduled_at"), time.Local)
	if err1 != nil || err2 != nil || err3 != nil {
		h.redirectWithError(w, r, patientID, practitionerID, "Please choose a practitioner, date, and slot.")
		return
	}

	// Re-derive the slot server-side rather than trusting client-submitted
	// duration/service-unit, and as a final staleness check.
	available, err := h.availableSlots(r, practitionerID, scheduledAt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var matched *Slot
	for i := range available {
		if available[i].Start.Equal(scheduledAt) {
			matched = &available[i]
			break
		}
	}
	if matched == nil {
		h.redirectWithError(w, r, patientID, practitionerID, "That slot is no longer available. Please pick another.")
		return
	}

	if _, err := h.PractitionerStore.Get(r.Context(), practitionerID); err != nil {
		http.NotFound(w, r)
		return
	}

	createdBy := auth.CurrentUserID(h.Sessions, r)
	_, err = h.Store.Create(r.Context(), Input{
		PatientID:       patientID,
		PractitionerID:  practitionerID,
		ServiceUnitID:   matched.ServiceUnitID,
		ScheduledAt:     scheduledAt,
		DurationMinutes: matched.DurationMinutes,
		Status:          "scheduled",
		Source:          "staff",
		AppointmentType: r.PostForm.Get("appointment_type"),
		ReasonForVisit:  r.PostForm.Get("reason_for_visit"),
		FeeAmount:       parseOptionalFloat(r.PostForm.Get("fee_amount")),
		CreatedBy:       &createdBy,
	})
	if err != nil {
		h.redirectWithError(w, r, patientID, practitionerID, "That slot is no longer available. Please pick another.")
		return
	}

	http.Redirect(w, r, "/appointments?practitioner_id="+strconv.FormatInt(practitionerID, 10)+
		"&date="+scheduledAt.Format(dateLayout), http.StatusSeeOther)
}

func (h *Handlers) redirectWithError(w http.ResponseWriter, r *http.Request, patientID, practitionerID int64, msg string) {
	target := "/appointments/new?patient_id=" + strconv.FormatInt(patientID, 10) +
		"&practitioner_id=" + strconv.FormatInt(practitionerID, 10) + "&error=" + url.QueryEscape(msg)
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// --- Status transitions ---

func (h *Handlers) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.Store.UpdateStatus(r.Context(), id, r.PostForm.Get("status")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	appt, err := h.Store.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/appointments?practitioner_id="+strconv.FormatInt(appt.PractitionerID, 10)+
		"&date="+appt.ScheduledAt.Format(dateLayout), http.StatusSeeOther)
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

// --- Fee capture (no invoice/ledger, just amount + paid/unpaid) ---

func (h *Handlers) SetFee(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	amount := parseOptionalFloat(r.PostForm.Get("fee_amount"))
	paid := r.PostForm.Get("fee_paid") == "on"

	if err := h.Store.SetFee(r.Context(), id, amount, paid); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	appt, err := h.Store.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/appointments?practitioner_id="+strconv.FormatInt(appt.PractitionerID, 10)+
		"&date="+appt.ScheduledAt.Format(dateLayout), http.StatusSeeOther)
}
