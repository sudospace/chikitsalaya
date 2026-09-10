package auth

import (
	"net/http"
	"slices"

	"github.com/alexedwards/scs/v2"
)

// RequireAuth redirects to /login unless the session has a logged-in user.
func RequireAuth(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !sm.Exists(r.Context(), sessionUserIDKey) {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequirePending2FA redirects to /login unless the session has passed the
// password check and is waiting on a TOTP code.
func RequirePending2FA(sm *scs.SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !sm.Exists(r.Context(), sessionPendingUserIDKey) {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole rejects the request with 403 unless the session's role is one
// of allowed. Must run after RequireAuth.
func RequireRole(sm *scs.SessionManager, allowed ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := sm.GetString(r.Context(), sessionRoleKey)
			if !slices.Contains(allowed, role) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireModule rejects the request with 403 unless the current user has
// at least requiredLevel access to module (see permissions.go). Must run
// after RequireAuth.
func RequireModule(sm *scs.SessionManager, permStore *PermissionStore, module, requiredLevel string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := CurrentRole(sm, r)
			userID := CurrentUserID(sm, r)
			ok, err := permStore.HasAccess(r.Context(), role, userID, module, requiredLevel)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if !ok {
				http.Error(w, "forbidden: you don't have access to this", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
