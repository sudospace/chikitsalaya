package company

import (
	"net/http"
	"strconv"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"

	"chikitsalaya/internal/web"
)

type Handlers struct {
	Store    *Store
	Sessions *scs.SessionManager
	Renderer *web.Renderer
}

func NewHandlers(store *Store, sm *scs.SessionManager, renderer *web.Renderer) *Handlers {
	return &Handlers{Store: store, Sessions: sm, Renderer: renderer}
}

type companyFormData struct {
	Company   *Company
	Action    string
	CancelURL string
	Error     string
}

// --- Clinic settings: the one clinic, always the same row ---

func (h *Handlers) MyCompany(w http.ResponseWriter, r *http.Request) {
	c, err := h.Store.GetClinic(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.Render(w, r, "company_form.html", companyFormData{Company: c, Action: "/admin/company", CancelURL: "/"})
}

func (h *Handlers) UpdateMyCompany(w http.ResponseWriter, r *http.Request) {
	in, err := parseCompanyForm(r)
	if err != nil {
		existing, _ := h.Store.GetClinic(r.Context())
		h.Renderer.Render(w, r, "company_form.html", companyFormData{Company: existing, Action: "/admin/company", CancelURL: "/", Error: err.Error()})
		return
	}
	if err := h.Store.UpdateClinic(r.Context(), in); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	c, err := h.Store.GetClinic(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.Store.SaveLogoIfProvided(r, c.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.Store.SaveLetterheadImageIfProvided(r, c.ID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/admin/company", http.StatusSeeOther)
}

func parseCompanyForm(r *http.Request) (CompanyInput, error) {
	// Multipart: the form also carries optional logo/letterhead files;
	// ParseMultipartForm populates r.PostForm for the regular fields too.
	if err := r.ParseMultipartForm(2*maxLogoBytes + 1<<20); err != nil {
		return CompanyInput{}, err
	}
	letterheadMode := r.PostForm.Get("letterhead_mode")
	if letterheadMode != "image" {
		letterheadMode = "html" // anything else (unset, tampered) defaults safely — the CHECK constraint only allows these two
	}
	return CompanyInput{
		Name:                 r.PostForm.Get("display_name"),
		Address:              r.PostForm.Get("address"),
		Phone:                r.PostForm.Get("phone"),
		Email:                r.PostForm.Get("email"),
		RegistrationNo:       r.PostForm.Get("registration_no"),
		LetterheadMode:       letterheadMode,
		LetterheadHTML:       r.PostForm.Get("letterhead_html"),
		FooterText:           r.PostForm.Get("footer_text"),
		FirstConsultationFee: parseOptionalFloat(r.PostForm.Get("first_consultation_fee")),
		FollowUpFee:          parseOptionalFloat(r.PostForm.Get("follow_up_fee")),
		GSTIN:                r.PostForm.Get("gstin"),
		GSTRegistered:        r.PostForm.Get("gst_registered") == "on",
		FYStartMonth:         parseFYStartMonth(r.PostForm.Get("fy_start_month")),
	}, nil
}

// parseFYStartMonth falls back to April (the DB column's own default) for
// anything unset, tampered, or out of the 1-12 range.
func parseFYStartMonth(v string) int {
	m, err := strconv.Atoi(v)
	if err != nil || m < 1 || m > 12 {
		return 4
	}
	return m
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

// --- Service units: the clinic's rooms/physical units ---

func (h *Handlers) ListServiceUnits(w http.ResponseWriter, r *http.Request) {
	units, err := h.Store.ListServiceUnits(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.Render(w, r, "service_units_list.html", units)
}

type serviceUnitFormData struct {
	Unit   *ServiceUnit
	Action string
	Error  string
}

func (h *Handlers) NewServiceUnit(w http.ResponseWriter, r *http.Request) {
	h.Renderer.Render(w, r, "service_unit_form.html", serviceUnitFormData{Unit: &ServiceUnit{}, Action: "/admin/service-units"})
}

func (h *Handlers) CreateServiceUnit(w http.ResponseWriter, r *http.Request) {
	in, err := parseServiceUnitForm(r)
	if err != nil {
		h.Renderer.Render(w, r, "service_unit_form.html", serviceUnitFormData{Unit: &ServiceUnit{}, Action: "/admin/service-units", Error: err.Error()})
		return
	}
	if _, err := h.Store.CreateServiceUnit(r.Context(), in); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/service-units", http.StatusSeeOther)
}

func (h *Handlers) EditServiceUnit(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	u, err := h.Store.GetServiceUnit(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.Renderer.Render(w, r, "service_unit_form.html", serviceUnitFormData{Unit: u, Action: "/admin/service-units/" + strconv.FormatInt(id, 10)})
}

func (h *Handlers) UpdateServiceUnit(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	in, err := parseServiceUnitForm(r)
	if err != nil {
		existing, _ := h.Store.GetServiceUnit(r.Context(), id)
		h.Renderer.Render(w, r, "service_unit_form.html", serviceUnitFormData{
			Unit: existing, Action: "/admin/service-units/" + strconv.FormatInt(id, 10), Error: err.Error(),
		})
		return
	}
	if err := h.Store.UpdateServiceUnit(r.Context(), id, in); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/service-units", http.StatusSeeOther)
}

func parseServiceUnitForm(r *http.Request) (ServiceUnitInput, error) {
	if err := r.ParseForm(); err != nil {
		return ServiceUnitInput{}, err
	}
	return ServiceUnitInput{
		Name:     r.PostForm.Get("name"),
		UnitType: r.PostForm.Get("unit_type"),
		IsActive: r.PostForm.Get("is_active") == "on",
	}, nil
}

func idParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}
