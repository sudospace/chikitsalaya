package practitioner

import (
	"net/http"
	"strconv"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"

	"chikitsalaya/internal/auth"
	"chikitsalaya/internal/opd/company"
	"chikitsalaya/internal/opd/schedule"
	"chikitsalaya/internal/web"
)

type Handlers struct {
	Store         *Store
	ScheduleStore *schedule.Store
	CompanyStore  *company.Store
	Sessions      *scs.SessionManager
	Renderer      *web.Renderer
}

func NewHandlers(store *Store, scheduleStore *schedule.Store, companyStore *company.Store, sm *scs.SessionManager, renderer *web.Renderer) *Handlers {
	return &Handlers{Store: store, ScheduleStore: scheduleStore, CompanyStore: companyStore, Sessions: sm, Renderer: renderer}
}

type listData struct {
	Practitioners []Practitioner
	IsAdmin       bool // gates the "New Practitioner" link
}

func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	practitioners, err := h.Store.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	isAdmin := auth.CurrentRole(h.Sessions, r) == auth.RoleAdmin
	h.Renderer.Render(w, r, "practitioners_list.html", listData{Practitioners: practitioners, IsAdmin: isAdmin})
}

type formData struct {
	Practitioner    *Practitioner
	Action          string
	Error           string
	Specializations []string
}

const manageBase = "/admin/practitioners-manage"

// specializations is a fixed, curated list for the dropdown; an existing
// practitioner's value is always included even if not on this list.
var specializations = []string{
	"General Medicine", "Family Medicine", "Pediatrics", "Gynecology & Obstetrics",
	"Dermatology", "Orthopedics", "ENT (Otolaryngology)", "Ophthalmology",
	"Cardiology", "Neurology", "Psychiatry", "Dentistry", "Urology", "Nephrology",
	"Gastroenterology", "Endocrinology", "Pulmonology", "Oncology", "Rheumatology",
	"General Surgery", "Plastic Surgery", "Physiotherapy", "Dietetics & Nutrition",
	"Ayurveda", "Homeopathy", "Other",
}

func specializationOptions(current string) []string {
	if current == "" {
		return specializations
	}
	for _, s := range specializations {
		if s == current {
			return specializations
		}
	}
	return append([]string{current}, specializations...)
}

func (h *Handlers) New(w http.ResponseWriter, r *http.Request) {
	h.Renderer.Render(w, r, "practitioner_form.html", formData{
		Practitioner: &Practitioner{}, Action: manageBase, Specializations: specializations,
	})
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	in, err := parseForm(r)
	if err != nil {
		h.Renderer.Render(w, r, "practitioner_form.html", formData{
			Practitioner: &Practitioner{}, Action: manageBase, Error: err.Error(), Specializations: specializations,
		})
		return
	}
	id, err := h.Store.Create(r.Context(), in)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/practitioners/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

type detailData struct {
	Practitioner *Practitioner
	Schedules    []schedule.Schedule
	ServiceUnits []company.ServiceUnit
}

func (h *Handlers) Detail(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p, err := h.Store.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	schedules, err := h.ScheduleStore.ListByPractitioner(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	units, err := h.CompanyStore.ListServiceUnits(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.Render(w, r, "practitioner_detail.html", detailData{
		Practitioner: p, Schedules: schedules, ServiceUnits: units,
	})
}

func (h *Handlers) Edit(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p, err := h.Store.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.Renderer.Render(w, r, "practitioner_form.html", formData{
		Practitioner: p, Action: manageBase + "/" + strconv.FormatInt(id, 10), Specializations: specializationOptions(p.Specialization),
	})
}

func (h *Handlers) Update(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	in, err := parseForm(r)
	if err != nil {
		existing, _ := h.Store.Get(r.Context(), id)
		h.Renderer.Render(w, r, "practitioner_form.html", formData{
			Practitioner: existing, Action: manageBase + "/" + strconv.FormatInt(id, 10), Error: err.Error(),
			Specializations: specializationOptions(r.PostForm.Get("specialization")),
		})
		return
	}
	if err := h.Store.Update(r.Context(), id, in); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/practitioners/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func idParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func parseForm(r *http.Request) (Input, error) {
	if err := r.ParseForm(); err != nil {
		return Input{}, err
	}
	return Input{
		FullName:       r.PostForm.Get("full_name"),
		Qualification:  r.PostForm.Get("qualification"),
		Specialization: r.PostForm.Get("specialization"),
		Phone:          r.PostForm.Get("phone"),
		Email:          r.PostForm.Get("email"),
		IsActive:       r.PostForm.Get("is_active") == "on",
	}, nil
}
