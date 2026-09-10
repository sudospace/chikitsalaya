package auth

import (
	"html/template"
	"net/http"
)

type setupPageData struct {
	// template.URL, not string: a plain string here gets silently replaced
	// with "#ZgotmplZ" by html/template's sanitizer, since data: URIs aren't
	// on its safe-scheme allowlist. Safe to trust — we generated this PNG
	// ourselves from the user's own TOTP secret.
	QRCodeDataURI template.URL
	Secret        string
	Error         string
}

// TwoFASetupPage shows the enrollment QR code, reusing the pending user's
// secret across reloads so refreshing doesn't invalidate an already-scanned code.
func (h *Handlers) TwoFASetupPage(w http.ResponseWriter, r *http.Request) {
	user, err := h.Store.GetByID(r.Context(), PendingUserID(h.Sessions, r))
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if user.TOTPConfirmed() {
		http.Redirect(w, r, "/2fa/verify", http.StatusSeeOther)
		return
	}

	var secret string
	if user.TOTPSecret != nil {
		secret = *user.TOTPSecret
	} else {
		key, err := GenerateTOTPSecret(user.Email)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		secret = key.Secret()
		if err := h.Store.SetTOTPSecret(r.Context(), user.ID, secret); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	qrURI, err := TOTPQRCodeDataURI(otpauthURL(user.Email, secret))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.Renderer.Render(w, r, "2fa_setup.html", setupPageData{QRCodeDataURI: template.URL(qrURI), Secret: secret})
}

func (h *Handlers) TwoFASetupConfirm(w http.ResponseWriter, r *http.Request) {
	user, err := h.Store.GetByID(r.Context(), PendingUserID(h.Sessions, r))
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if user.TOTPSecret == nil {
		http.Redirect(w, r, "/2fa/setup", http.StatusSeeOther)
		return
	}
	code := r.PostForm.Get("code")
	if !ValidateTOTPCode(code, *user.TOTPSecret) {
		qrURI, _ := TOTPQRCodeDataURI(otpauthURL(user.Email, *user.TOTPSecret))
		h.Renderer.Render(w, r, "2fa_setup.html", setupPageData{
			QRCodeDataURI: template.URL(qrURI), Secret: *user.TOTPSecret, Error: "That code didn't match. Try again.",
		})
		return
	}
	if err := h.Store.EnableTOTP(r.Context(), user.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	target, err := h.promoteSession(r, user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

type verifyPageData struct {
	Error string
}

func (h *Handlers) TwoFAVerifyPage(w http.ResponseWriter, r *http.Request) {
	h.Renderer.Render(w, r, "2fa_verify.html", verifyPageData{})
}

func (h *Handlers) TwoFAVerifyConfirm(w http.ResponseWriter, r *http.Request) {
	user, err := h.Store.GetByID(r.Context(), PendingUserID(h.Sessions, r))
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if user.TOTPSecret == nil || !user.TOTPConfirmed() {
		http.Redirect(w, r, "/2fa/setup", http.StatusSeeOther)
		return
	}
	code := r.PostForm.Get("code")
	if !ValidateTOTPCode(code, *user.TOTPSecret) {
		h.Renderer.Render(w, r, "2fa_verify.html", verifyPageData{Error: "That code didn't match. Try again."})
		return
	}
	target, err := h.promoteSession(r, user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}
