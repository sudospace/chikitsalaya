package patient

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"

	"chikitsalaya/internal/auth"
	"chikitsalaya/internal/opd/document"
	"chikitsalaya/internal/web"
)

type Handlers struct {
	Store           *Store
	DocumentStore   *document.Store
	PermissionStore *auth.PermissionStore
	Sessions        *scs.SessionManager
	Renderer        *web.Renderer
}

func NewHandlers(store *Store, documentStore *document.Store, permissionStore *auth.PermissionStore, sm *scs.SessionManager, renderer *web.Renderer) *Handlers {
	return &Handlers{Store: store, DocumentStore: documentStore, PermissionStore: permissionStore, Sessions: sm, Renderer: renderer}
}

type listData struct {
	Query    string
	Patients []Patient
}

func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	patients, err := h.Store.List(r.Context(), q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.Render(w, r, "patients_list.html", listData{Query: q, Patients: patients})
}

// Search is the htmx endpoint backing live search: it returns just the
// table-row fragment, not a full page.
func (h *Handlers) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	patients, err := h.Store.List(r.Context(), q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.RenderPartial(w, "patient_rows", patients)
}

var bloodGroups = []string{"A+", "A-", "B+", "B-", "AB+", "AB-", "O+", "O-"}

type formData struct {
	Patient     *Patient
	Action      string
	Error       string
	BloodGroups []string
}

func (h *Handlers) New(w http.ResponseWriter, r *http.Request) {
	h.Renderer.Render(w, r, "patient_form.html", formData{Patient: &Patient{}, Action: "/patients", BloodGroups: bloodGroups})
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	in, err := parseForm(r)
	if err != nil {
		h.Renderer.Render(w, r, "patient_form.html", formData{Patient: &Patient{}, Action: "/patients", Error: err.Error(), BloodGroups: bloodGroups})
		return
	}
	id, err := h.Store.Create(r.Context(), in)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/patients/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

type detailData struct {
	Patient          *Patient
	Documents        []document.Document
	CanEditDocuments bool
	Error            string
}

func (h *Handlers) Detail(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	p, err := h.Store.Get(ctx, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	docs, err := h.DocumentStore.ListByPatient(ctx, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	role := auth.CurrentRole(h.Sessions, r)
	userID := auth.CurrentUserID(h.Sessions, r)
	canEdit, _ := h.PermissionStore.HasAccess(ctx, role, userID, auth.ModulePatientDocuments, auth.AccessEdit)

	h.Renderer.Render(w, r, "patient_detail.html", detailData{
		Patient: p, Documents: docs, CanEditDocuments: canEdit, Error: r.URL.Query().Get("error"),
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
	h.Renderer.Render(w, r, "patient_form.html", formData{
		Patient:     p,
		Action:      "/patients/" + strconv.FormatInt(id, 10),
		BloodGroups: bloodGroups,
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
		h.Renderer.Render(w, r, "patient_form.html", formData{
			Patient:     existing,
			Action:      "/patients/" + strconv.FormatInt(id, 10),
			Error:       err.Error(),
			BloodGroups: bloodGroups,
		})
		return
	}
	if err := h.Store.Update(r.Context(), id, in); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/patients/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func idParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func parseForm(r *http.Request) (Input, error) {
	if err := r.ParseForm(); err != nil {
		return Input{}, err
	}
	var age *int
	if v := r.PostForm.Get("age"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Input{}, err
		}
		age = &n
	}
	if r.PostForm.Get("first_name") == "" {
		return Input{}, errors.New("first name is required")
	}
	return Input{
		FirstName:             r.PostForm.Get("first_name"),
		LastName:              r.PostForm.Get("last_name"),
		Age:                   age,
		Sex:                   r.PostForm.Get("sex"),
		Phone:                 r.PostForm.Get("phone"),
		Email:                 r.PostForm.Get("email"),
		Address:               r.PostForm.Get("address"),
		EmergencyContactName:  r.PostForm.Get("emergency_contact_name"),
		EmergencyContactPhone: r.PostForm.Get("emergency_contact_phone"),
		BloodGroup:            r.PostForm.Get("blood_group"),
		Allergies:             r.PostForm.Get("allergies"),
		MedicalHistory:        r.PostForm.Get("medical_history"),
	}, nil
}
