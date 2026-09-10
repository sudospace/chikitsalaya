// Package dashboard implements the post-login home page and the header's global search.
package dashboard

import (
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"

	"chikitsalaya/internal/auth"
	"chikitsalaya/internal/opd/appointment"
	"chikitsalaya/internal/opd/company"
	"chikitsalaya/internal/opd/patient"
	"chikitsalaya/internal/opd/practitioner"
	"chikitsalaya/internal/web"
)

// todayFetchLimit backs both the preview and the quick-stat counts, so
// undercounting here would make the stats wrong, not just trim the preview.
const todayFetchLimit = 200

const todayPreviewRows = 6
const searchResultLimit = 6

type Handlers struct {
	AppointmentStore  *appointment.Store
	CompanyStore      *company.Store
	PatientStore      *patient.Store
	PractitionerStore *practitioner.Store
	PermissionStore   *auth.PermissionStore
	UserStore         *auth.Store
	Sessions          *scs.SessionManager
	Renderer          *web.Renderer
}

func NewHandlers(appointmentStore *appointment.Store, companyStore *company.Store, patientStore *patient.Store,
	practitionerStore *practitioner.Store, permissionStore *auth.PermissionStore, userStore *auth.Store,
	sm *scs.SessionManager, renderer *web.Renderer) *Handlers {
	return &Handlers{
		AppointmentStore: appointmentStore, CompanyStore: companyStore, PatientStore: patientStore,
		PractitionerStore: practitionerStore, PermissionStore: permissionStore, UserStore: userStore, Sessions: sm, Renderer: renderer,
	}
}

// todayStats is derived from the same fetch as TodayPreview so the two never disagree.
type todayStats struct {
	Total          int
	NeedsAttention int // "requested" — online bookings awaiting a Confirm
	CheckedIn      int
	Completed      int
}

type homeData struct {
	Company         *company.Company
	TodayPreview    []appointment.Appointment
	MoreToday       int // precomputed: html/template has no arithmetic function
	Stats           todayStats
	OwnScheduleOnly bool // true when narrowed to the signed-in doctor's own day
	IsAdmin         bool
}

func (h *Handlers) Home(w http.ResponseWriter, r *http.Request) {
	role := auth.CurrentRole(h.Sessions, r)

	data := homeData{
		IsAdmin: role == auth.RoleAdmin,
	}

	c, err := h.CompanyStore.GetClinic(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data.Company = c

	// A doctor sees their own day; front desk still needs the clinic-wide view.
	var appts []appointment.Appointment
	var practitionerID *int64
	if role == auth.RoleDoctor || auth.IsActingAsDoctor(h.Sessions, r) {
		if u, err := h.UserStore.GetByID(r.Context(), auth.CurrentUserID(h.Sessions, r)); err == nil {
			practitionerID = u.PractitionerID
		}
	}
	if practitionerID != nil {
		data.OwnScheduleOnly = true
		all, err := h.AppointmentStore.ListByPractitionerAndDate(r.Context(), *practitionerID, time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, a := range all {
			if a.Status != "cancelled" && a.Status != "no_show" {
				appts = append(appts, a)
			}
		}
	} else {
		appts, err = h.AppointmentStore.ListToday(r.Context(), todayFetchLimit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	for _, a := range appts {
		switch a.Status {
		case "requested":
			data.Stats.NeedsAttention++
		case "checked_in":
			data.Stats.CheckedIn++
		case "completed":
			data.Stats.Completed++
		}
	}
	data.Stats.Total = len(appts)
	if len(appts) > todayPreviewRows {
		data.MoreToday = len(appts) - todayPreviewRows
		appts = appts[:todayPreviewRows]
	}
	data.TodayPreview = appts

	h.Renderer.Render(w, r, "home.html", data)
}

type searchResults struct {
	Query         string
	Patients      []patient.Patient
	Practitioners []practitioner.Practitioner
}

// Search only includes a category if the user has view access to that
// module — otherwise search would defeat module restrictions.
func (h *Handlers) Search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		h.Renderer.RenderPartial(w, "global_search_results", searchResults{})
		return
	}

	role := auth.CurrentRole(h.Sessions, r)
	userID := auth.CurrentUserID(h.Sessions, r)
	ctx := r.Context()

	results := searchResults{Query: q}

	if canPatients, err := h.PermissionStore.HasAccess(ctx, role, userID, auth.ModulePatients, auth.AccessView); err == nil && canPatients {
		patients, err := h.PatientStore.List(ctx, q)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(patients) > searchResultLimit {
			patients = patients[:searchResultLimit]
		}
		results.Patients = patients
	}

	if canPractitioners, err := h.PermissionStore.HasAccess(ctx, role, userID, auth.ModulePractitioners, auth.AccessView); err == nil && canPractitioners {
		all, err := h.PractitionerStore.List(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		qLower := strings.ToLower(q)
		var matched []practitioner.Practitioner
		for _, p := range all {
			if strings.Contains(strings.ToLower(p.FullName), qLower) {
				matched = append(matched, p)
				if len(matched) == searchResultLimit {
					break
				}
			}
		}
		results.Practitioners = matched
	}

	h.Renderer.RenderPartial(w, "global_search_results", results)
}
