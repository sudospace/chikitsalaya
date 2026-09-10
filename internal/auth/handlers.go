package auth

import (
	"net/http"

	"github.com/alexedwards/scs/v2"

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

type loginPageData struct {
	Error string
}

func (h *Handlers) LoginPage(w http.ResponseWriter, r *http.Request) {
	h.Renderer.Render(w, r, "login.html", loginPageData{})
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	email := r.PostForm.Get("email")
	password := r.PostForm.Get("password")

	user, err := h.Store.Authenticate(r.Context(), email, password)
	if err != nil {
		h.Renderer.Render(w, r, "login.html", loginPageData{Error: "Invalid email or password."})
		return
	}

	// RenewToken guards against session fixation before the session holds anything sensitive.
	if err := h.Sessions.RenewToken(r.Context()); err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	h.Sessions.Put(r.Context(), sessionPendingUserIDKey, int(user.ID))

	if user.TOTPConfirmed() {
		http.Redirect(w, r, "/2fa/verify", http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/2fa/setup", http.StatusSeeOther)
	}
}

// promoteSession turns a password-verified-but-not-yet-2FA'd session into a
// fully authenticated one, called after a correct TOTP code. Returns the
// redirect target (the onboarding wizard on a first login, home otherwise).
func (h *Handlers) promoteSession(r *http.Request, user *User) (string, error) {
	if err := h.Sessions.RenewToken(r.Context()); err != nil {
		return "", err
	}
	h.Sessions.Remove(r.Context(), sessionPendingUserIDKey)
	h.Sessions.Put(r.Context(), sessionUserIDKey, int(user.ID))
	h.Sessions.Put(r.Context(), sessionRoleKey, user.Role)

	if user.OnboardedAt == nil && user.Role == RoleAdmin {
		return "/onboarding", nil
	}
	return "/", nil
}

// PendingUserID returns the id of a user who has passed the password check
// but not yet 2FA, or 0 if there isn't one.
func PendingUserID(sm *scs.SessionManager, r *http.Request) int64 {
	return int64(sm.GetInt(r.Context(), sessionPendingUserIDKey))
}

func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	if err := h.Sessions.Destroy(r.Context()); err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// CurrentUserID returns the logged-in user's id, or 0 if there isn't one.
func CurrentUserID(sm *scs.SessionManager, r *http.Request) int64 {
	return int64(sm.GetInt(r.Context(), sessionUserIDKey))
}

// CurrentRole returns the logged-in user's role, or "" if there isn't one.
func CurrentRole(sm *scs.SessionManager, r *http.Request) string {
	return sm.GetString(r.Context(), sessionRoleKey)
}

// SetActingAsDoctor switches an admin into their linked practitioner's
// doctor identity. CurrentRole and /admin route gates are untouched.
func SetActingAsDoctor(sm *scs.SessionManager, r *http.Request) {
	sm.Put(r.Context(), sessionActingAsDoctorKey, true)
}

// ClearActingAsDoctor exits doctor-acting view, back to the plain admin view.
func ClearActingAsDoctor(sm *scs.SessionManager, r *http.Request) {
	sm.Remove(r.Context(), sessionActingAsDoctorKey)
}

// IsActingAsDoctor reports whether the current session is an admin
// currently switched into their own linked doctor identity.
func IsActingAsDoctor(sm *scs.SessionManager, r *http.Request) bool {
	return sm.GetBool(r.Context(), sessionActingAsDoctorKey)
}
