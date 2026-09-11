package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Module keys a login can be granted access to. Hardcoded rather than
// database-driven — adding one means a constant here plus a route guard.
const (
	ModulePatients        = "patients"
	ModulePractitioners   = "practitioners"
	ModuleAppointments    = "appointments"
	ModuleEncounters      = "encounters"
	ModuleCompanySettings = "company_settings"
	ModuleIntegrations    = "integrations"
	ModuleServiceUnits    = "service_units"
	ModuleDrugs           = "drugs"
	ModuleLookupValues    = "lookup_values"
	ModuleLabTests        = "lab_tests"
	ModuleProcedures      = "procedures"
	ModuleUsers           = "users"
	ModuleBilling         = "billing"
	ModuleReports         = "reports"
)

const (
	AccessNone = ""
	AccessView = "view"
	AccessEdit = "edit"
)

// ModuleInfo pairs a module key with its human label, in a stable order —
// used to render the permission checklist on the user-creation form.
type ModuleInfo struct {
	Key   string
	Label string
}

var AllModules = []ModuleInfo{
	{ModulePatients, "Patients"},
	{ModulePractitioners, "Practitioners"},
	{ModuleAppointments, "Appointments"},
	{ModuleEncounters, "Consultations"},
	{ModuleCompanySettings, "My Clinic settings"},
	{ModuleIntegrations, "Integrations"},
	{ModuleServiceUnits, "Service Units"},
	{ModuleDrugs, "Drugs"},
	{ModuleLookupValues, "Dosage/Duration"},
	{ModuleLabTests, "Lab Tests"},
	{ModuleProcedures, "Procedures"},
	{ModuleUsers, "Users"},
	{ModuleBilling, "Billing"},
	{ModuleReports, "Reports & Stats"},
}

// StaffDefaultPermissions is pre-checked on a doctor/receptionist creation
// form: full clinical access, read-only practitioner directory, no admin
// modules. Note: this map isn't role-specific, so Billing's inclusion here
// (wanted for receptionist, per the checkout workflow) also pre-checks it
// for a freshly created doctor — there's no per-role default map to split
// it out further; an admin can still uncheck it per-user on the form.
var StaffDefaultPermissions = map[string]string{
	ModulePatients:      AccessEdit,
	ModulePractitioners: AccessView,
	ModuleAppointments:  AccessEdit,
	ModuleEncounters:    AccessEdit,
	ModuleBilling:       AccessEdit,
}

// AdminDefaultPermissions is pre-checked for a freshly created admin: full
// edit access everywhere.
var AdminDefaultPermissions = func() map[string]string {
	m := make(map[string]string, len(AllModules))
	for _, mod := range AllModules {
		m[mod.Key] = AccessEdit
	}
	return m
}()

type PermissionStore struct {
	Pool *pgxpool.Pool
}

func NewPermissionStore(pool *pgxpool.Pool) *PermissionStore {
	return &PermissionStore{Pool: pool}
}

// Get returns a user's module -> access_level map. A module missing from
// the result means no access at all, not an error.
func (s *PermissionStore) Get(ctx context.Context, userID int64) (map[string]string, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT module, access_level FROM user_module_permissions WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var module, level string
		if err := rows.Scan(&module, &level); err != nil {
			return nil, err
		}
		out[module] = level
	}
	return out, rows.Err()
}

// SetAll replaces every permission row for a user with perms in one
// transaction — called once at creation time, and whenever an admin edits
// a user's access afterward. A module absent from perms (or set to
// AccessNone) simply gets no row, i.e. no access.
func (s *PermissionStore) SetAll(ctx context.Context, userID int64, perms map[string]string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM user_module_permissions WHERE user_id = $1`, userID); err != nil {
		return err
	}
	for module, level := range perms {
		if level != AccessView && level != AccessEdit {
			continue
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO user_module_permissions (user_id, module, access_level) VALUES ($1, $2, $3)`,
			userID, module, level); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// HasAccess reports whether a user can reach module at requiredLevel.
// requiredLevel AccessView is satisfied by either a "view" or "edit" row;
// AccessEdit requires an "edit" row.
func (s *PermissionStore) HasAccess(ctx context.Context, role string, userID int64, module, requiredLevel string) (bool, error) {
	var level string
	err := s.Pool.QueryRow(ctx,
		`SELECT access_level FROM user_module_permissions WHERE user_id = $1 AND module = $2`,
		userID, module).Scan(&level)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if requiredLevel == AccessView {
		return level == AccessView || level == AccessEdit, nil
	}
	return level == AccessEdit, nil
}
