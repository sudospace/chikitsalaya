package auth

import (
	"net/http"
	"time"

	"github.com/alexedwards/scs/pgxstore"
	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

const sessionUserIDKey = "userID"
const sessionRoleKey = "role"

// sessionActingAsDoctorKey flags that an admin has switched into viewing
// their own linked practitioner's doctor dashboard.
const sessionActingAsDoctorKey = "actingAsDoctor"

// sessionPendingUserIDKey holds a user id between password and TOTP checks;
// RequireAuth ignores it, so this state has no access beyond /2fa/*.
const sessionPendingUserIDKey = "pending2FAUserID"

// NewSessionManager wires up scs with a Postgres-backed store.
func NewSessionManager(pool *pgxpool.Pool) *scs.SessionManager {
	sm := scs.New()
	sm.Store = pgxstore.New(pool)
	sm.Lifetime = 12 * time.Hour
	sm.Cookie.HttpOnly = true
	sm.Cookie.SameSite = http.SameSiteLaxMode
	return sm
}
