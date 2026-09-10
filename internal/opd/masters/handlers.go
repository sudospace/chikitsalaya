package masters

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

// --- Lookup values ---

func (h *Handlers) ListLookupValues(w http.ResponseWriter, r *http.Request) {
	values, err := h.Store.ListLookupValues(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.Render(w, r, "lookup_values_list.html", values)
}

type lookupFormData struct {
	Value      *LookupValue
	Categories []string
	Action     string
	Error      string
}

func (h *Handlers) NewLookupValue(w http.ResponseWriter, r *http.Request) {
	h.Renderer.Render(w, r, "lookup_value_form.html", lookupFormData{
		Value: &LookupValue{}, Categories: LookupCategories, Action: "/admin/lookup-values",
	})
}

func (h *Handlers) CreateLookupValue(w http.ResponseWriter, r *http.Request) {
	in, err := parseLookupForm(r)
	if err != nil {
		h.Renderer.Render(w, r, "lookup_value_form.html", lookupFormData{
			Value: &LookupValue{}, Categories: LookupCategories, Action: "/admin/lookup-values", Error: err.Error(),
		})
		return
	}
	if _, err := h.Store.CreateLookupValue(r.Context(), in); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/lookup-values", http.StatusSeeOther)
}

func (h *Handlers) EditLookupValue(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	v, err := h.Store.GetLookupValue(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.Renderer.Render(w, r, "lookup_value_form.html", lookupFormData{
		Value: v, Categories: LookupCategories, Action: "/admin/lookup-values/" + strconv.FormatInt(id, 10),
	})
}

func (h *Handlers) UpdateLookupValue(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	in, err := parseLookupForm(r)
	if err != nil {
		existing, _ := h.Store.GetLookupValue(r.Context(), id)
		h.Renderer.Render(w, r, "lookup_value_form.html", lookupFormData{
			Value: existing, Categories: LookupCategories,
			Action: "/admin/lookup-values/" + strconv.FormatInt(id, 10), Error: err.Error(),
		})
		return
	}
	if err := h.Store.UpdateLookupValue(r.Context(), id, in); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/lookup-values", http.StatusSeeOther)
}

func (h *Handlers) DeleteLookupValue(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.Store.DeleteLookupValue(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/lookup-values", http.StatusSeeOther)
}

func parseLookupForm(r *http.Request) (LookupValueInput, error) {
	if err := r.ParseForm(); err != nil {
		return LookupValueInput{}, err
	}
	sortOrder, err := strconv.Atoi(r.PostForm.Get("sort_order"))
	if err != nil {
		sortOrder = 0
	}
	return LookupValueInput{
		Category:  r.PostForm.Get("category"),
		Value:     r.PostForm.Get("value"),
		SortOrder: sortOrder,
	}, nil
}

// --- Lab test templates ---

func (h *Handlers) ListLabTestTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := h.Store.ListLabTestTemplates(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.Render(w, r, "lab_test_templates_list.html", templates)
}

type labTestFormData struct {
	Template *LabTestTemplate
	Action   string
	Error    string
}

func (h *Handlers) NewLabTestTemplate(w http.ResponseWriter, r *http.Request) {
	h.Renderer.Render(w, r, "lab_test_template_form.html", labTestFormData{Template: &LabTestTemplate{}, Action: "/admin/lab-tests"})
}

func (h *Handlers) CreateLabTestTemplate(w http.ResponseWriter, r *http.Request) {
	in, err := parseLabTestForm(r)
	if err != nil {
		h.Renderer.Render(w, r, "lab_test_template_form.html", labTestFormData{Template: &LabTestTemplate{}, Action: "/admin/lab-tests", Error: err.Error()})
		return
	}
	if existing, err := h.Store.FindLabTestTemplateByName(r.Context(), in.Name); err == nil {
		h.Renderer.Render(w, r, "lab_test_template_form.html", labTestFormData{
			Template: &LabTestTemplate{Name: in.Name}, Action: "/admin/lab-tests",
			Error: "A lab test named \"" + existing.Name + "\" already exists in this clinic's catalog.",
		})
		return
	}
	if _, err := h.Store.CreateLabTestTemplate(r.Context(), in); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/lab-tests", http.StatusSeeOther)
}

func (h *Handlers) EditLabTestTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	t, err := h.Store.GetLabTestTemplate(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.Renderer.Render(w, r, "lab_test_template_form.html", labTestFormData{Template: t, Action: "/admin/lab-tests/" + strconv.FormatInt(id, 10)})
}

func (h *Handlers) UpdateLabTestTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	in, err := parseLabTestForm(r)
	if err != nil {
		existing, _ := h.Store.GetLabTestTemplate(r.Context(), id)
		h.Renderer.Render(w, r, "lab_test_template_form.html", labTestFormData{
			Template: existing, Action: "/admin/lab-tests/" + strconv.FormatInt(id, 10), Error: err.Error(),
		})
		return
	}
	if err := h.Store.UpdateLabTestTemplate(r.Context(), id, in); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/lab-tests", http.StatusSeeOther)
}

func parseLabTestForm(r *http.Request) (LabTestTemplateInput, error) {
	if err := r.ParseForm(); err != nil {
		return LabTestTemplateInput{}, err
	}
	price, err := parsePrice(r.PostForm.Get("price"))
	if err != nil {
		return LabTestTemplateInput{}, err
	}
	return LabTestTemplateInput{
		Name:       r.PostForm.Get("name"),
		Department: r.PostForm.Get("department"),
		Price:      price,
		SampleType: r.PostForm.Get("sample_type"),
		IsActive:   r.PostForm.Get("is_active") == "on",
	}, nil
}

// --- Procedure templates ---

func (h *Handlers) ListProcedureTemplates(w http.ResponseWriter, r *http.Request) {
	templates, err := h.Store.ListProcedureTemplates(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.Render(w, r, "procedure_templates_list.html", templates)
}

type procedureFormData struct {
	Template *ProcedureTemplate
	Action   string
	Error    string
}

func (h *Handlers) NewProcedureTemplate(w http.ResponseWriter, r *http.Request) {
	h.Renderer.Render(w, r, "procedure_template_form.html", procedureFormData{Template: &ProcedureTemplate{}, Action: "/admin/procedures"})
}

func (h *Handlers) CreateProcedureTemplate(w http.ResponseWriter, r *http.Request) {
	in, err := parseProcedureForm(r)
	if err != nil {
		h.Renderer.Render(w, r, "procedure_template_form.html", procedureFormData{Template: &ProcedureTemplate{}, Action: "/admin/procedures", Error: err.Error()})
		return
	}
	if _, err := h.Store.CreateProcedureTemplate(r.Context(), in); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/procedures", http.StatusSeeOther)
}

func (h *Handlers) EditProcedureTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	t, err := h.Store.GetProcedureTemplate(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h.Renderer.Render(w, r, "procedure_template_form.html", procedureFormData{Template: t, Action: "/admin/procedures/" + strconv.FormatInt(id, 10)})
}

func (h *Handlers) UpdateProcedureTemplate(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	in, err := parseProcedureForm(r)
	if err != nil {
		existing, _ := h.Store.GetProcedureTemplate(r.Context(), id)
		h.Renderer.Render(w, r, "procedure_template_form.html", procedureFormData{
			Template: existing, Action: "/admin/procedures/" + strconv.FormatInt(id, 10), Error: err.Error(),
		})
		return
	}
	if err := h.Store.UpdateProcedureTemplate(r.Context(), id, in); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/procedures", http.StatusSeeOther)
}

func parseProcedureForm(r *http.Request) (ProcedureTemplateInput, error) {
	if err := r.ParseForm(); err != nil {
		return ProcedureTemplateInput{}, err
	}
	price, err := parsePrice(r.PostForm.Get("price"))
	if err != nil {
		return ProcedureTemplateInput{}, err
	}
	return ProcedureTemplateInput{
		Name:        r.PostForm.Get("name"),
		Department:  r.PostForm.Get("department"),
		Price:       price,
		Description: r.PostForm.Get("description"),
		IsActive:    r.PostForm.Get("is_active") == "on",
	}, nil
}

func parsePrice(v string) (*float64, error) {
	if v == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func idParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}
