// Package booking implements the public, unauthenticated online booking
// page (/book). A visitor identifies by mobile + OTP, then books or manages
// an appointment, which lands as status='requested' until staff confirm it.
package booking

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"chikitsalaya/internal/opd/appointment"
	"chikitsalaya/internal/opd/company"
	"chikitsalaya/internal/opd/integrations"
	"chikitsalaya/internal/opd/notify"
	"chikitsalaya/internal/opd/patient"
	"chikitsalaya/internal/opd/practitioner"
	"chikitsalaya/internal/opd/schedule"
	"chikitsalaya/internal/web"
)

const dateLayout = "2006-01-02"
const slotValueLayout = "2006-01-02T15:04"

// sessionVerifiedMobileKey holds the OTP-verified mobile for this anonymous session.
const sessionVerifiedMobileKey = "bookingVerifiedMobile"

type Handlers struct {
	CompanyStore      *company.Store
	PractitionerStore *practitioner.Store
	ScheduleStore     *schedule.Store
	AppointmentStore  *appointment.Store
	PatientStore      *patient.Store
	OTPStore          *OTPStore
	IntegrationsStore *integrations.Store
	SMS               notify.SMSSender
	Sessions          *scs.SessionManager
	Renderer          *web.Renderer
}

func NewHandlers(companyStore *company.Store, practitionerStore *practitioner.Store, scheduleStore *schedule.Store,
	appointmentStore *appointment.Store, patientStore *patient.Store, otpStore *OTPStore, integrationsStore *integrations.Store,
	sms notify.SMSSender, sm *scs.SessionManager, renderer *web.Renderer) *Handlers {
	return &Handlers{
		CompanyStore: companyStore, PractitionerStore: practitionerStore, ScheduleStore: scheduleStore,
		AppointmentStore: appointmentStore, PatientStore: patientStore, OTPStore: otpStore,
		IntegrationsStore: integrationsStore, SMS: sms, Sessions: sm, Renderer: renderer,
	}
}

// resolveSMSSender falls back to the app-wide default SMS sender unless
// the clinic has configured and enabled its own.
func (h *Handlers) resolveSMSSender(ctx context.Context) notify.SMSSender {
	in, err := h.IntegrationsStore.Get(ctx, integrations.ModuleSMS)
	if err != nil || in == nil || !in.IsActive || in.Provider != "webhook" {
		return h.SMS
	}
	return notify.WebhookSender{
		URL:            in.Config["webhook_url"],
		AuthHeaderName: in.Config["auth_header_name"],
		AuthHeaderVal:  in.Config["auth_header_value"],
	}
}

func (h *Handlers) verifiedMobile(r *http.Request) string {
	return h.Sessions.GetString(r.Context(), sessionVerifiedMobileKey)
}

// verifiedPatient returns nil, nil if verified but no patient record exists yet.
func (h *Handlers) verifiedPatient(r *http.Request) (*patient.Patient, error) {
	mobile := h.verifiedMobile(r)
	if mobile == "" {
		return nil, nil
	}
	p, err := h.PatientStore.FindByPhone(r.Context(), mobile)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

type pageData struct {
	Company        *company.Company
	Verified       bool
	Mobile         string
	OTPRequested   bool
	Error          string
	Info           string
	Patient        *patient.Patient
	Upcoming       []appointment.Appointment
	Past           []appointment.Appointment
	Practitioners  []practitioner.Practitioner
	PractitionerID int64
	Date           string
}

// Index renders the whole single-page flow: an identify step (mobile +
// OTP) if the session isn't verified yet, else the manage step (existing
// appointments + a new-booking form).
func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	c, err := h.CompanyStore.GetClinic(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := pageData{Company: c, Date: dateParam(r)}
	if msg := r.URL.Query().Get("error"); msg != "" {
		data.Error = msg
	}
	if msg := r.URL.Query().Get("info"); msg != "" {
		data.Info = msg
	}

	mobile := h.verifiedMobile(r)
	if mobile == "" {
		if v := r.URL.Query().Get("otp_sent_to"); v != "" {
			data.Mobile = v
			data.OTPRequested = true
		}
		h.Renderer.RenderStandalone(w, "book.html", data)
		return
	}

	data.Verified = true
	data.Mobile = mobile
	pat, err := h.verifiedPatient(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data.Patient = pat
	if pat != nil {
		appts, err := h.AppointmentStore.ListByPatient(r.Context(), pat.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		now := time.Now()
		for _, a := range appts {
			if a.Status != "cancelled" && a.Status != "no_show" && a.ScheduledAt.After(now) {
				data.Upcoming = append(data.Upcoming, a)
			} else {
				data.Past = append(data.Past, a)
			}
		}
	}

	practitioners, err := h.PractitionerStore.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data.Practitioners = practitioners
	if pidStr := r.URL.Query().Get("practitioner_id"); pidStr != "" {
		if pid, err := strconv.ParseInt(pidStr, 10, 64); err == nil {
			data.PractitionerID = pid
		}
	}

	h.Renderer.RenderStandalone(w, "book.html", data)
}

func dateParam(r *http.Request) string {
	if v := r.URL.Query().Get("date"); v != "" {
		return v
	}
	return time.Now().Format(dateLayout)
}

// --- OTP identify step ---

func (h *Handlers) RequestOTP(w http.ResponseWriter, r *http.Request) {
	c, err := h.CompanyStore.GetClinic(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	mobile := r.PostForm.Get("mobile")
	if mobile == "" {
		h.redirectIdentify(w, r, "Please enter your mobile number.")
		return
	}
	code, err := h.OTPStore.CreateOTP(r.Context(), mobile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sender := h.resolveSMSSender(r.Context())
	if err := sender.Send(mobile, "Your "+c.Name+" verification code is "+code); err != nil {
		h.redirectIdentify(w, r, "We couldn't send the verification code. Please try again in a moment.")
		return
	}

	http.Redirect(w, r, "/book?otp_sent_to="+url.QueryEscape(mobile), http.StatusSeeOther)
}

func (h *Handlers) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	mobile := r.PostForm.Get("mobile")
	code := r.PostForm.Get("code")

	ok, err := h.OTPStore.VerifyOTP(r.Context(), mobile, code)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok {
		target := "/book?otp_sent_to=" + url.QueryEscape(mobile) +
			"&error=" + url.QueryEscape("That code is incorrect or has expired.")
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}

	if err := h.Sessions.RenewToken(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Sessions.Put(r.Context(), sessionVerifiedMobileKey, mobile)
	http.Redirect(w, r, "/book", http.StatusSeeOther)
}

// Logout drops the verified-mobile session so another number can be checked.
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	h.Sessions.Remove(r.Context(), sessionVerifiedMobileKey)
	http.Redirect(w, r, "/book", http.StatusSeeOther)
}

func (h *Handlers) redirectIdentify(w http.ResponseWriter, r *http.Request, msg string) {
	http.Redirect(w, r, "/book?error="+url.QueryEscape(msg), http.StatusSeeOther)
}

// --- Slot grid (shared by new booking + reschedule) ---

type slotsData struct {
	Slots []appointment.Slot
}

func (h *Handlers) Slots(w http.ResponseWriter, r *http.Request) {
	practitionerID, err := strconv.ParseInt(r.URL.Query().Get("practitioner_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid practitioner_id", http.StatusBadRequest)
		return
	}
	if _, err := h.PractitionerStore.Get(r.Context(), practitionerID); err != nil {
		http.NotFound(w, r)
		return
	}
	date, err := time.ParseInLocation(dateLayout, r.URL.Query().Get("date"), time.Local)
	if err != nil {
		http.Error(w, "invalid date", http.StatusBadRequest)
		return
	}

	available, err := h.availableSlots(r, practitionerID, date)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.RenderPartial(w, "appointment_slots", slotsData{Slots: available})
}

func (h *Handlers) availableSlots(r *http.Request, practitionerID int64, date time.Time) ([]appointment.Slot, error) {
	schedules, err := h.ScheduleStore.ListByPractitioner(r.Context(), practitionerID)
	if err != nil {
		return nil, err
	}
	generated, err := appointment.GenerateDaySlots(schedules, date)
	if err != nil {
		return nil, err
	}
	booked, err := h.AppointmentStore.BookedRanges(r.Context(), practitionerID, date)
	if err != nil {
		return nil, err
	}
	return appointment.AvailableSlots(generated, booked), nil
}

// --- New booking (requires a verified session) ---

// Create takes the phone number from the verified session, never a form
// field — nothing typed can book under an unproven number.
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	mobile := h.verifiedMobile(r)
	if mobile == "" {
		h.redirectIdentify(w, r, "Please verify your mobile number first.")
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	practitionerID, err1 := strconv.ParseInt(r.PostForm.Get("practitioner_id"), 10, 64)
	scheduledAt, err2 := time.ParseInLocation(slotValueLayout, r.PostForm.Get("scheduled_at"), time.Local)
	if err1 != nil || err2 != nil {
		h.redirectManage(w, r, practitionerID, "Please choose a doctor, date, and time.")
		return
	}
	if _, err := h.PractitionerStore.Get(r.Context(), practitionerID); err != nil {
		http.NotFound(w, r)
		return
	}

	available, err := h.availableSlots(r, practitionerID, scheduledAt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var matched *appointment.Slot
	for i := range available {
		if available[i].Start.Equal(scheduledAt) {
			matched = &available[i]
			break
		}
	}
	if matched == nil {
		h.redirectManage(w, r, practitionerID, "That slot is no longer available. Please pick another.")
		return
	}

	patientID, err := h.findOrCreatePatient(r, mobile)
	if err != nil {
		h.redirectManage(w, r, practitionerID, err.Error())
		return
	}

	_, err = h.AppointmentStore.Create(r.Context(), appointment.Input{
		PatientID:       patientID,
		PractitionerID:  practitionerID,
		ServiceUnitID:   matched.ServiceUnitID,
		ScheduledAt:     scheduledAt,
		DurationMinutes: matched.DurationMinutes,
		Status:          "requested",
		Source:          "online",
		ReasonForVisit:  r.PostForm.Get("reason_for_visit"),
		CreatedBy:       nil,
	})
	if err != nil {
		h.redirectManage(w, r, practitionerID, "That slot is no longer available. Please pick another.")
		return
	}

	http.Redirect(w, r, "/book?info="+url.QueryEscape("Your appointment request has been sent. Our team will confirm it shortly."), http.StatusSeeOther)
}

// findOrCreatePatient creates a minimal patient record for a first-time visitor.
func (h *Handlers) findOrCreatePatient(r *http.Request, mobile string) (int64, error) {
	existing, err := h.PatientStore.FindByPhone(r.Context(), mobile)
	if err == nil {
		return existing.ID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}

	firstName := r.PostForm.Get("first_name")
	if firstName == "" {
		return 0, errors.New("please enter your name")
	}
	var age *int
	if v := r.PostForm.Get("age"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			age = &n
		}
	}
	return h.PatientStore.Create(r.Context(), patient.Input{
		FirstName: firstName,
		LastName:  r.PostForm.Get("last_name"),
		Age:       age,
		Sex:       r.PostForm.Get("gender"),
		Phone:     mobile,
	})
}

func (h *Handlers) redirectManage(w http.ResponseWriter, r *http.Request, practitionerID int64, msg string) {
	target := "/book?error=" + url.QueryEscape(msg)
	if practitionerID != 0 {
		target += "&practitioner_id=" + strconv.FormatInt(practitionerID, 10)
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// --- Cancel / reschedule an existing appointment (ownership-checked) ---

func (h *Handlers) Cancel(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	pat, err := h.verifiedPatient(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if pat == nil {
		h.redirectIdentify(w, r, "Please verify your mobile number first.")
		return
	}
	if err := h.AppointmentStore.CancelOwned(r.Context(), id, pat.ID); err != nil {
		http.Redirect(w, r, "/book?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/book?info="+url.QueryEscape("Your appointment has been cancelled."), http.StatusSeeOther)
}

type rescheduleData struct {
	Company     *company.Company
	Appointment *appointment.Appointment
	Date        string
	Error       string
}

// RescheduleForm shows a slot picker scoped to the appointment's existing
// doctor (a visitor reschedules the time, not who they're seeing).
func (h *Handlers) RescheduleForm(w http.ResponseWriter, r *http.Request) {
	c, appt, ok := h.ownedAppointment(w, r)
	if !ok {
		return
	}
	h.Renderer.RenderStandalone(w, "book_reschedule.html", rescheduleData{
		Company: c, Appointment: appt, Date: dateParam(r),
		Error: r.URL.Query().Get("error"),
	})
}

// RescheduleSlots is like Slots but the practitioner is fixed, not client-supplied.
func (h *Handlers) RescheduleSlots(w http.ResponseWriter, r *http.Request) {
	_, appt, ok := h.ownedAppointment(w, r)
	if !ok {
		return
	}
	date, err := time.ParseInLocation(dateLayout, r.URL.Query().Get("date"), time.Local)
	if err != nil {
		http.Error(w, "invalid date", http.StatusBadRequest)
		return
	}
	available, err := h.availableSlots(r, appt.PractitionerID, date)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.RenderPartial(w, "appointment_slots", slotsData{Slots: available})
}

func (h *Handlers) Reschedule(w http.ResponseWriter, r *http.Request) {
	_, appt, ok := h.ownedAppointment(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	scheduledAt, err := time.ParseInLocation(slotValueLayout, r.PostForm.Get("scheduled_at"), time.Local)
	if err != nil {
		http.Redirect(w, r, "/book/appointments/"+strconv.FormatInt(appt.ID, 10)+"/reschedule?error="+
			url.QueryEscape("Please pick a time."), http.StatusSeeOther)
		return
	}

	available, err := h.availableSlots(r, appt.PractitionerID, scheduledAt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var matched *appointment.Slot
	for i := range available {
		if available[i].Start.Equal(scheduledAt) {
			matched = &available[i]
			break
		}
	}
	if matched == nil {
		http.Redirect(w, r, "/book/appointments/"+strconv.FormatInt(appt.ID, 10)+"/reschedule?error="+
			url.QueryEscape("That slot is no longer available. Please pick another."), http.StatusSeeOther)
		return
	}

	if err := h.AppointmentStore.Reschedule(r.Context(), appt.ID, appt.PatientID, scheduledAt, matched.DurationMinutes, matched.ServiceUnitID); err != nil {
		http.Redirect(w, r, "/book/appointments/"+strconv.FormatInt(appt.ID, 10)+"/reschedule?error="+
			url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/book?info="+url.QueryEscape("Your appointment has been moved and is pending confirmation."), http.StatusSeeOther)
}

// ownedAppointment writes a 404/redirect itself and returns ok=false if the
// appointment doesn't belong to the session's verified patient.
func (h *Handlers) ownedAppointment(w http.ResponseWriter, r *http.Request) (*company.Company, *appointment.Appointment, bool) {
	c, err := h.CompanyStore.GetClinic(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, nil, false
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return nil, nil, false
	}
	pat, err := h.verifiedPatient(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, nil, false
	}
	if pat == nil {
		h.redirectIdentify(w, r, "Please verify your mobile number first.")
		return nil, nil, false
	}
	appt, err := h.AppointmentStore.Get(r.Context(), id)
	if err != nil || appt.PatientID != pat.ID {
		http.NotFound(w, r)
		return nil, nil, false
	}
	return c, appt, true
}
