// Package onboarding implements the one-time first-login wizard: the admin
// setting up practitioners/drugs/lab tests/procedures for the clinic.
package onboarding

import (
	"net/http"

	"github.com/alexedwards/scs/v2"

	"chikitsalaya/internal/auth"
	"chikitsalaya/internal/opd/inventory"
	"chikitsalaya/internal/opd/masters"
	"chikitsalaya/internal/opd/practitioner"
	"chikitsalaya/internal/opd/schedule"
	"chikitsalaya/internal/web"
)

type Handlers struct {
	UserStore         *auth.Store
	PractitionerStore *practitioner.Store
	ScheduleStore     *schedule.Store
	InventoryStore    *inventory.Store
	MastersStore      *masters.Store
	Sessions          *scs.SessionManager
	Renderer          *web.Renderer
}

func NewHandlers(userStore *auth.Store, practitionerStore *practitioner.Store,
	scheduleStore *schedule.Store, inventoryStore *inventory.Store, mastersStore *masters.Store,
	sm *scs.SessionManager, renderer *web.Renderer) *Handlers {
	return &Handlers{
		UserStore: userStore, PractitionerStore: practitionerStore,
		ScheduleStore: scheduleStore, InventoryStore: inventoryStore, MastersStore: mastersStore,
		Sessions: sm, Renderer: renderer,
	}
}

// Index sends any not-yet-onboarded admin into the wizard (only admin
// logins ever reach this route — see auth.promoteSession).
func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/onboarding/practitioners", http.StatusSeeOther)
}

// Skip marks the current user onboarded without completing the wizard, so nobody gets stuck.
func (h *Handlers) Skip(w http.ResponseWriter, r *http.Request) {
	if err := h.UserStore.MarkOnboarded(r.Context(), auth.CurrentUserID(h.Sessions, r)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
