// Package staff implements the "create a login" screen: any admin creating
// another admin/doctor/receptionist login for the clinic.
package staff

import (
	"net/http"
	"strconv"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"

	"chikitsalaya/internal/auth"
	"chikitsalaya/internal/opd/practitioner"
	"chikitsalaya/internal/web"
)

type Handlers struct {
	UserStore         *auth.Store
	PermissionStore   *auth.PermissionStore
	PractitionerStore *practitioner.Store
	Sessions          *scs.SessionManager
	Renderer          *web.Renderer
}

func NewHandlers(userStore *auth.Store, permissionStore *auth.PermissionStore,
	practitionerStore *practitioner.Store, sm *scs.SessionManager, renderer *web.Renderer) *Handlers {
	return &Handlers{
		UserStore: userStore, PermissionStore: permissionStore,
		PractitionerStore: practitionerStore, Sessions: sm, Renderer: renderer,
	}
}

// parsePermissionsForm reads one perm_<module> select per module. Anything
// other than "view"/"edit" (a tampered value) is dropped, i.e. no access.
func parsePermissionsForm(r *http.Request) map[string]string {
	perms := make(map[string]string, len(auth.AllModules))
	for _, m := range auth.AllModules {
		v := r.PostForm.Get("perm_" + m.Key)
		if v == auth.AccessView || v == auth.AccessEdit {
			perms[m.Key] = v
		}
	}
	return perms
}

func idParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

// --- Create logins for the clinic: admin, doctor, or receptionist ---

func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	users, err := h.UserStore.ListAll(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.Render(w, r, "admin_users_list.html", users)
}

type adminUserFormData struct {
	Practitioners []practitioner.Practitioner
	Error         string
	Modules       []auth.ModuleInfo
	Permissions   map[string]string
}

func (h *Handlers) New(w http.ResponseWriter, r *http.Request) {
	practitioners, err := h.PractitionerStore.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.Render(w, r, "admin_user_form.html", adminUserFormData{
		Practitioners: practitioners, Modules: auth.AllModules, Permissions: auth.StaffDefaultPermissions,
	})
}

var allowedRoles = map[string]bool{auth.RoleAdmin: true, auth.RoleDoctor: true, auth.RoleReceptionist: true}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	role := r.PostForm.Get("role")
	fullName := r.PostForm.Get("full_name")
	phone := r.PostForm.Get("phone")
	email := r.PostForm.Get("email")
	password := r.PostForm.Get("password")
	if !allowedRoles[role] || fullName == "" || email == "" || password == "" {
		h.renderFormError(w, r, "Please provide a full name, email, password, and a valid role.")
		return
	}
	if password != r.PostForm.Get("confirm_password") {
		h.renderFormError(w, r, "Password and confirmation don't match.")
		return
	}

	var practitionerID *int64
	if role == auth.RoleDoctor {
		// "existing" is set only when the admin explicitly links a second
		// login to a practitioner already on the roster.
		if r.PostForm.Get("practitioner_mode") == "existing" {
			v := r.PostForm.Get("practitioner_id")
			if v == "" {
				h.renderFormError(w, r, "Please choose a practitioner to link, or switch to \"Create a new practitioner\".")
				return
			}
			id, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				h.renderFormError(w, r, "Invalid practitioner selection.")
				return
			}
			practitionerID = &id
		} else {
			newID, err := h.PractitionerStore.Create(r.Context(), practitioner.Input{
				FullName: fullName, Email: email, Phone: phone,
				Qualification:  r.PostForm.Get("practitioner_qualification"),
				Specialization: r.PostForm.Get("practitioner_specialization"),
				IsActive:       true,
			})
			if err != nil {
				h.renderFormError(w, r, "Couldn't create the practitioner record: "+err.Error())
				return
			}
			practitionerID = &newID
		}
	}

	id, err := h.UserStore.Create(r.Context(), auth.CreateInput{
		Email: email, Password: password, Role: role, PractitionerID: practitionerID,
		FullName: fullName, Phone: phone, Address: r.PostForm.Get("address"),
		EmergencyContactName:  r.PostForm.Get("emergency_contact_name"),
		EmergencyContactPhone: r.PostForm.Get("emergency_contact_phone"),
	})
	if err != nil {
		h.renderFormError(w, r, err.Error())
		return
	}
	if err := h.PermissionStore.SetAll(r.Context(), id, parsePermissionsForm(r)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (h *Handlers) renderFormError(w http.ResponseWriter, r *http.Request, msg string) {
	practitioners, _ := h.PractitionerStore.List(r.Context())
	h.Renderer.Render(w, r, "admin_user_form.html", adminUserFormData{
		Practitioners: practitioners, Error: msg, Modules: auth.AllModules, Permissions: parsePermissionsForm(r),
	})
}

// ToggleActive activates/deactivates a login.
func (h *Handlers) ToggleActive(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	active := r.PostForm.Get("active") == "true"
	if err := h.UserStore.SetActive(r.Context(), id, active); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

type adminUserEditData struct {
	User                *auth.User
	PractitionerEditURL string // set only for a doctor with a linked practitioner
	Error               string
}

// Edit lets an admin correct one of the clinic's logins' personal details.
func (h *Handlers) Edit(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	u, err := h.UserStore.GetByID(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	data := adminUserEditData{User: u}
	if u.Role == auth.RoleDoctor && u.PractitionerID != nil {
		data.PractitionerEditURL = "/admin/practitioners-manage/" + strconv.FormatInt(*u.PractitionerID, 10) + "/edit"
	}
	h.Renderer.Render(w, r, "admin_user_edit.html", data)
}

func (h *Handlers) Update(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if _, err := h.UserStore.GetByID(r.Context(), id); err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	in := auth.PersonalInput{
		FullName: r.PostForm.Get("full_name"), Phone: r.PostForm.Get("phone"),
		Address:               r.PostForm.Get("address"),
		EmergencyContactName:  r.PostForm.Get("emergency_contact_name"),
		EmergencyContactPhone: r.PostForm.Get("emergency_contact_phone"),
	}
	if err := h.UserStore.UpdatePersonal(r.Context(), id, in); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}
