package inventory

import (
	"net/http"
	"strconv"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"

	"chikitsalaya/internal/auth"
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

func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	drugs, err := h.Store.ListDrugs(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.Render(w, r, "drugs_list.html", drugs)
}

type formData struct {
	Drug   *Drug
	Action string
	Error  string
}

func (h *Handlers) New(w http.ResponseWriter, r *http.Request) {
	h.Renderer.Render(w, r, "drug_form.html", formData{Drug: &Drug{}, Action: "/admin/drugs"})
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	in, err := parseForm(r)
	if err != nil {
		h.Renderer.Render(w, r, "drug_form.html", formData{Drug: &Drug{}, Action: "/admin/drugs", Error: err.Error()})
		return
	}
	if existing, err := h.Store.FindDrugByName(r.Context(), in.Name); err == nil {
		h.Renderer.Render(w, r, "drug_form.html", formData{
			Drug: &Drug{Name: in.Name}, Action: "/admin/drugs",
			Error: "A drug named \"" + existing.Name + "\" already exists in this clinic's catalog.",
		})
		return
	}
	if _, err := h.Store.CreateDrug(r.Context(), in, auth.CurrentUserID(h.Sessions, r)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/drugs", http.StatusSeeOther)
}

func (h *Handlers) Edit(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	d, err := h.Store.GetDrug(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.Renderer.Render(w, r, "drug_form.html", formData{Drug: d, Action: "/admin/drugs/" + strconv.FormatInt(id, 10)})
}

func (h *Handlers) Update(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	in, err := parseForm(r)
	if err != nil {
		existing, _ := h.Store.GetDrug(r.Context(), id)
		h.Renderer.Render(w, r, "drug_form.html", formData{
			Drug: existing, Action: "/admin/drugs/" + strconv.FormatInt(id, 10), Error: err.Error(),
		})
		return
	}
	if err := h.Store.UpdateDrug(r.Context(), id, in); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/drugs", http.StatusSeeOther)
}

func idParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func parseForm(r *http.Request) (DrugInput, error) {
	if err := r.ParseForm(); err != nil {
		return DrugInput{}, err
	}
	var reorder *float64
	if v := r.PostForm.Get("reorder_level"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return DrugInput{}, err
		}
		reorder = &f
	}
	var opening float64
	if v := r.PostForm.Get("opening_stock"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return DrugInput{}, err
		}
		opening = f
	}
	return DrugInput{
		Name:         r.PostForm.Get("name"),
		GenericName:  r.PostForm.Get("generic_name"),
		Form:         r.PostForm.Get("form"),
		Strength:     r.PostForm.Get("strength"),
		Unit:         r.PostForm.Get("unit"),
		ReorderLevel: reorder,
		IsActive:     r.PostForm.Get("is_active") == "on",
		OpeningStock: opening,
	}, nil
}
