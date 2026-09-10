package encounter

import (
	"context"
	"net/http"
)

// catalogHit is the uniform id+label shape every catalog search renders,
// so one partial template can handle matches from any catalog.
type catalogHit struct {
	ID    int64
	Label string
}

// catalogSearchData carries a search's hits plus enough context for
// catalog_search_results.html to render an inline "+ Add" affordance when
// nothing matched. QuickAddEndpoint is always the id-free /catalog/* mount
// since these search handlers themselves are never mounted under an
// encounter id (see cmd/chikitsalaya's route table).
type catalogSearchData struct {
	Query            string
	Kind             string // "drug" | "lab test" | "procedure"
	QuickAddEndpoint string
	QuickAddField    string // form field name the quick-add endpoint expects
	Hits             []catalogHit
}

// minSearchLen re-enforces the client's debounce threshold server-side.
const minSearchLen = 3

func (h *Handlers) searchDrugHits(ctx context.Context, q string) ([]catalogHit, error) {
	if len(q) < minSearchLen {
		return nil, nil
	}
	drugs, err := h.InventoryStore.SearchDrugs(ctx, q)
	if err != nil {
		return nil, err
	}
	var hits []catalogHit
	for _, d := range drugs {
		label := d.Name
		if d.Strength != "" {
			label += " (" + d.Strength + ")"
		}
		hits = append(hits, catalogHit{ID: d.ID, Label: label})
	}
	return hits, nil
}

func (h *Handlers) searchLabTestHits(ctx context.Context, q string) ([]catalogHit, error) {
	if len(q) < minSearchLen {
		return nil, nil
	}
	templates, err := h.MastersStore.SearchLabTestTemplates(ctx, q)
	if err != nil {
		return nil, err
	}
	var hits []catalogHit
	for _, t := range templates {
		hits = append(hits, catalogHit{ID: t.ID, Label: t.Name})
	}
	return hits, nil
}

func (h *Handlers) searchProcedureHits(ctx context.Context, q string) ([]catalogHit, error) {
	if len(q) < minSearchLen {
		return nil, nil
	}
	templates, err := h.MastersStore.SearchProcedureTemplates(ctx, q)
	if err != nil {
		return nil, err
	}
	var hits []catalogHit
	for _, t := range templates {
		hits = append(hits, catalogHit{ID: t.ID, Label: t.Name})
	}
	return hits, nil
}

// SearchDrugsHandler is the live search-as-you-type endpoint for the drug field.
func (h *Handlers) SearchDrugsHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	hits, err := h.searchDrugHits(r.Context(), q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.RenderPartial(w, "catalog_search_results", catalogSearchData{
		Query: q, Kind: "drug", QuickAddEndpoint: "/catalog/quick-add-drug", QuickAddField: "drug_name", Hits: hits,
	})
}

// SearchLabTestsHandler is the equivalent live search for lab test names.
func (h *Handlers) SearchLabTestsHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	hits, err := h.searchLabTestHits(r.Context(), q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.RenderPartial(w, "catalog_search_results", catalogSearchData{
		Query: q, Kind: "lab test", QuickAddEndpoint: "/catalog/quick-add-lab-test", QuickAddField: "test_name", Hits: hits,
	})
}

// SearchProceduresHandler is the equivalent live search for procedure names.
func (h *Handlers) SearchProceduresHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	hits, err := h.searchProcedureHits(r.Context(), q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.RenderPartial(w, "catalog_search_results", catalogSearchData{
		Query: q, Kind: "procedure", QuickAddEndpoint: "/catalog/quick-add-procedure", QuickAddField: "procedure_name", Hits: hits,
	})
}
