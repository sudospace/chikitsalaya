package schedule

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type Handlers struct {
	Store *Store
}

func NewHandlers(store *Store) *Handlers {
	return &Handlers{Store: store}
}

// Create adds a schedule block for the practitioner named in the URL and
// redirects back to that practitioner's detail page.
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	practitionerID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	dayOfWeek, err := strconv.Atoi(r.PostForm.Get("day_of_week"))
	if err != nil {
		http.Error(w, "invalid day_of_week", http.StatusBadRequest)
		return
	}
	slotDuration, err := strconv.Atoi(r.PostForm.Get("slot_duration_minutes"))
	if err != nil || slotDuration <= 0 {
		http.Error(w, "invalid slot_duration_minutes", http.StatusBadRequest)
		return
	}
	var serviceUnitID *int64
	if v := r.PostForm.Get("service_unit_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			http.Error(w, "invalid service_unit_id", http.StatusBadRequest)
			return
		}
		serviceUnitID = &id
	}

	_, err = h.Store.Create(r.Context(), Input{
		PractitionerID:      practitionerID,
		DayOfWeek:           dayOfWeek,
		StartTime:           r.PostForm.Get("start_time"),
		EndTime:             r.PostForm.Get("end_time"),
		SlotDurationMinutes: slotDuration,
		ServiceUnitID:       serviceUnitID,
		IsActive:            true,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/practitioners/"+strconv.FormatInt(practitionerID, 10), http.StatusSeeOther)
}

// Delete removes one schedule block and redirects back to the
// practitioner's detail page.
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	practitionerID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	scheduleID, err := strconv.ParseInt(chi.URLParam(r, "scheduleID"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := h.Store.Delete(r.Context(), scheduleID, practitionerID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/practitioners/"+strconv.FormatInt(practitionerID, 10), http.StatusSeeOther)
}
