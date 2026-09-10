// Package account implements the self-service "My Account" page: a user's
// own credentials, personal details, and (for a doctor) weekly schedule.
package account

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"chikitsalaya/internal/auth"
	"chikitsalaya/internal/opd/company"
	"chikitsalaya/internal/opd/practitioner"
	"chikitsalaya/internal/opd/schedule"
	"chikitsalaya/internal/web"
)

// roleLabels turns the stored role slug into what this page shows a
// person — nowhere else needs a human label for a role.
var roleLabels = map[string]string{
	auth.RoleAdmin:        "Admin",
	auth.RoleDoctor:       "Doctor",
	auth.RoleReceptionist: "Receptionist",
}

type Handlers struct {
	UserStore         *auth.Store
	PermissionStore   *auth.PermissionStore
	PractitionerStore *practitioner.Store
	ScheduleStore     *schedule.Store
	CompanyStore      *company.Store
	Sessions          *scs.SessionManager
	Renderer          *web.Renderer
}

func NewHandlers(userStore *auth.Store, permissionStore *auth.PermissionStore, practitionerStore *practitioner.Store,
	scheduleStore *schedule.Store, companyStore *company.Store, sm *scs.SessionManager, renderer *web.Renderer) *Handlers {
	return &Handlers{
		UserStore: userStore, PermissionStore: permissionStore, PractitionerStore: practitionerStore,
		ScheduleStore: scheduleStore, CompanyStore: companyStore, Sessions: sm, Renderer: renderer,
	}
}

// accessLabels turns a stored access_level ("", "view", "edit") into what
// the account page shows for it.
var accessLabels = map[string]string{
	auth.AccessNone: "No access",
	auth.AccessView: "View only",
	auth.AccessEdit: "Full access",
}

// modulePermission is one row of the "what can I do" table on the
// account page's Credentials tab.
type modulePermission struct {
	Label  string
	Access string // human label, e.g. "Full access" — never a raw access_level
}

type pageData struct {
	Email                 string
	RoleLabel             string
	TOTPOn                bool
	FullName              string
	Phone                 string
	Address               string
	EmergencyContactName  string
	EmergencyContactPhone string
	Error                 string
	Saved                 string
	IsDoctor              bool
	Practitioner          *practitioner.Practitioner
	Schedules             []schedule.Schedule
	ServiceUnits          []company.ServiceUnit
	Days                  []string

	// Admin-only "also a doctor" switch (see auth.SetActingAsDoctor).
	IsAdmin            bool
	LinkedPractitioner *practitioner.Practitioner // nil until admin links one
	ActingAsDoctor     bool
	AllPractitioners   []practitioner.Practitioner // link picker, only loaded when not yet linked

	Permissions []modulePermission
}

var weekdayNames = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

func (h *Handlers) Page(w http.ResponseWriter, r *http.Request) {
	u, err := h.UserStore.GetByID(r.Context(), auth.CurrentUserID(h.Sessions, r))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := pageData{
		Email: u.Email, RoleLabel: roleLabels[u.Role], TOTPOn: u.TOTPConfirmed(),
		FullName: u.FullName, Phone: u.Phone, Address: u.Address,
		EmergencyContactName: u.EmergencyContactName, EmergencyContactPhone: u.EmergencyContactPhone, Days: weekdayNames,
		IsAdmin: u.Role == auth.RoleAdmin, ActingAsDoctor: auth.IsActingAsDoctor(h.Sessions, r),
	}
	if data.RoleLabel == "" {
		data.RoleLabel = u.Role
	}
	if msg := r.URL.Query().Get("saved"); msg != "" {
		data.Saved = msg
	}
	if msg := r.URL.Query().Get("error"); msg != "" {
		data.Error = msg
	}

	if perms, err := h.PermissionStore.Get(r.Context(), u.ID); err == nil {
		for _, mod := range auth.AllModules {
			level := perms[mod.Key]
			label, ok := accessLabels[level]
			if !ok {
				label = accessLabels[auth.AccessNone]
			}
			data.Permissions = append(data.Permissions, modulePermission{Label: mod.Label, Access: label})
		}
	}

	isDoctorContext := u.Role == auth.RoleDoctor || data.ActingAsDoctor
	if isDoctorContext && u.PractitionerID != nil {
		pr, err := h.PractitionerStore.Get(r.Context(), *u.PractitionerID)
		if err == nil {
			data.IsDoctor = true
			data.Practitioner = pr
			if schedules, err := h.ScheduleStore.ListByPractitioner(r.Context(), *u.PractitionerID); err == nil {
				data.Schedules = schedules
			}
			if units, err := h.CompanyStore.ListServiceUnits(r.Context()); err == nil {
				data.ServiceUnits = units
			}
		}
	}

	if data.IsAdmin {
		if u.PractitionerID != nil {
			if pr, err := h.PractitionerStore.Get(r.Context(), *u.PractitionerID); err == nil {
				data.LinkedPractitioner = pr
			}
		} else if all, err := h.PractitionerStore.List(r.Context()); err == nil {
			data.AllPractitioners = all
		}
	}

	h.Renderer.Render(w, r, "account.html", data)
}

// LinkPractitioner lets an admin pick an existing practitioner to also act
// as. Admin-only; anyone else hitting this 404s.
func (h *Handlers) LinkPractitioner(w http.ResponseWriter, r *http.Request) {
	userID := auth.CurrentUserID(h.Sessions, r)
	u, err := h.UserStore.GetByID(r.Context(), userID)
	if err != nil || u.Role != auth.RoleAdmin {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	practitionerID, err := strconv.ParseInt(r.PostForm.Get("practitioner_id"), 10, 64)
	if err != nil {
		h.redirectError(w, r, "credentials", "Please choose a valid practitioner.")
		return
	}
	if _, err := h.PractitionerStore.Get(r.Context(), practitionerID); err != nil {
		h.redirectError(w, r, "credentials", "That practitioner could not be found.")
		return
	}
	if err := h.UserStore.LinkPractitioner(r.Context(), userID, &practitionerID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/account?saved=Practitioner+linked.+You+can+now+switch+to+Doctor+view.#credentials", http.StatusSeeOther)
}

// ActAsDoctor switches an admin already linked to a practitioner (see
// LinkPractitioner) into that doctor identity for the rest of the session.
func (h *Handlers) ActAsDoctor(w http.ResponseWriter, r *http.Request) {
	u, err := h.UserStore.GetByID(r.Context(), auth.CurrentUserID(h.Sessions, r))
	if err != nil || u.Role != auth.RoleAdmin || u.PractitionerID == nil {
		http.NotFound(w, r)
		return
	}
	if _, err := h.PractitionerStore.Get(r.Context(), *u.PractitionerID); err != nil {
		http.NotFound(w, r)
		return
	}
	auth.SetActingAsDoctor(h.Sessions, r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// StopActingAsDoctor exits doctor view, back to the admin's own dashboard.
func (h *Handlers) StopActingAsDoctor(w http.ResponseWriter, r *http.Request) {
	auth.ClearActingAsDoctor(h.Sessions, r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// redirectError sends the user back to /account with an error, landing on
// the given tab.
func (h *Handlers) redirectError(w http.ResponseWriter, r *http.Request, tab, msg string) {
	http.Redirect(w, r, "/account?error="+url.QueryEscape(msg)+"#"+tab, http.StatusSeeOther)
}

// UpdatePassword changes the signed-in user's own password, after
// verifying they still know the current one.
func (h *Handlers) UpdatePassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	userID := auth.CurrentUserID(h.Sessions, r)
	u, err := h.UserStore.GetByID(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	current := r.PostForm.Get("current_password")
	newPassword := r.PostForm.Get("new_password")
	confirm := r.PostForm.Get("confirm_password")

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(current)); err != nil {
		h.redirectError(w, r, "credentials", "Current password is incorrect.")
		return
	}
	if len(newPassword) < 8 {
		h.redirectError(w, r, "credentials", "New password must be at least 8 characters.")
		return
	}
	if newPassword != confirm {
		h.redirectError(w, r, "credentials", "New password and confirmation don't match.")
		return
	}
	if err := h.UserStore.SetPassword(r.Context(), userID, newPassword); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/account?saved=Password+updated.#credentials", http.StatusSeeOther)
}

// UpdatePersonal saves the signed-in user's personal details. Name is
// settable only once, to keep "who did this" traceable elsewhere in the app.
func (h *Handlers) UpdatePersonal(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	userID := auth.CurrentUserID(h.Sessions, r)
	u, err := h.UserStore.GetByID(r.Context(), userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fullName := u.FullName
	if fullName == "" {
		fullName = strings.TrimSpace(r.PostForm.Get("full_name"))
	}

	in := auth.PersonalInput{
		FullName: fullName, Phone: r.PostForm.Get("phone"), Address: r.PostForm.Get("address"),
		EmergencyContactName:  r.PostForm.Get("emergency_contact_name"),
		EmergencyContactPhone: r.PostForm.Get("emergency_contact_phone"),
	}
	if err := h.UserStore.UpdatePersonal(r.Context(), userID, in); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/account?saved=Personal+details+updated.#personal", http.StatusSeeOther)
}

// requireOwnPractitioner resolves the signed-in doctor's own practitioner id,
// or writes a response and returns ok=false.
func (h *Handlers) requireOwnPractitioner(w http.ResponseWriter, r *http.Request) (practitionerID int64, ok bool) {
	u, err := h.UserStore.GetByID(r.Context(), auth.CurrentUserID(h.Sessions, r))
	isDoctorContext := u != nil && (u.Role == auth.RoleDoctor || auth.IsActingAsDoctor(h.Sessions, r))
	if err != nil || !isDoctorContext || u.PractitionerID == nil {
		http.NotFound(w, r)
		return 0, false
	}
	return *u.PractitionerID, true
}

// AddSchedule adds one weekly availability block to the signed-in
// doctor's own schedule.
func (h *Handlers) AddSchedule(w http.ResponseWriter, r *http.Request) {
	practitionerID, ok := h.requireOwnPractitioner(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dayOfWeek, err := strconv.Atoi(r.PostForm.Get("day_of_week"))
	if err != nil {
		h.redirectError(w, r, "schedule", "Please choose a valid day.")
		return
	}
	slotDuration, err := strconv.Atoi(r.PostForm.Get("slot_duration_minutes"))
	if err != nil || slotDuration <= 0 {
		h.redirectError(w, r, "schedule", "Please enter a valid slot length.")
		return
	}
	var serviceUnitID *int64
	if v := r.PostForm.Get("service_unit_id"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			serviceUnitID = &id
		}
	}

	_, err = h.ScheduleStore.Create(r.Context(), schedule.Input{
		PractitionerID: practitionerID, DayOfWeek: dayOfWeek,
		StartTime: r.PostForm.Get("start_time"), EndTime: r.PostForm.Get("end_time"),
		SlotDurationMinutes: slotDuration, ServiceUnitID: serviceUnitID, IsActive: true,
	})
	if err != nil {
		h.redirectError(w, r, "schedule", "Couldn't add that schedule block. Please try again.")
		return
	}
	http.Redirect(w, r, "/account?saved=Schedule+updated.#schedule", http.StatusSeeOther)
}

// DeleteSchedule removes one block from the signed-in doctor's own
// schedule.
func (h *Handlers) DeleteSchedule(w http.ResponseWriter, r *http.Request) {
	practitionerID, ok := h.requireOwnPractitioner(w, r)
	if !ok {
		return
	}
	scheduleID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.ScheduleStore.Delete(r.Context(), scheduleID, practitionerID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/account?saved=Schedule+updated.#schedule", http.StatusSeeOther)
}
