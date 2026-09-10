package integrations

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

type pageData struct {
	SMS     *Integration
	Email   *Integration
	Payment *Integration
	Saved   string
	Error   string
}

func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sms, err := h.Store.Get(ctx, ModuleSMS)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	email, err := h.Store.Get(ctx, ModuleEmail)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	payment, err := h.Store.Get(ctx, ModulePayment)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.Renderer.Render(w, r, "integrations.html", pageData{
		SMS: sms, Email: email, Payment: payment,
		Saved: r.URL.Query().Get("saved"), Error: r.URL.Query().Get("error"),
	})
}

// UpdateSMS supports only a generic outbound webhook (see notify.WebhookSender) — no vendor SDK.
func (h *Handlers) UpdateSMS(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	provider := r.PostForm.Get("provider")
	config := map[string]string{
		"webhook_url":       r.PostForm.Get("webhook_url"),
		"auth_header_name":  r.PostForm.Get("auth_header_name"),
		"auth_header_value": r.PostForm.Get("auth_header_value"),
		"sender_id":         r.PostForm.Get("sender_id"),
	}
	isActive := r.PostForm.Get("is_active") == "on"
	if err := h.Store.Upsert(r.Context(), ModuleSMS, provider, config, isActive); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/integrations?saved=sms", http.StatusSeeOther)
}

// UpdateEmail stores SMTP config only — nothing sends email yet.
func (h *Handlers) UpdateEmail(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	provider := r.PostForm.Get("provider")
	config := map[string]string{
		"smtp_host":    r.PostForm.Get("smtp_host"),
		"smtp_port":    r.PostForm.Get("smtp_port"),
		"username":     r.PostForm.Get("username"),
		"password":     r.PostForm.Get("password"),
		"from_address": r.PostForm.Get("from_address"),
	}
	isActive := r.PostForm.Get("is_active") == "on"
	if err := h.Store.Upsert(r.Context(), ModuleEmail, provider, config, isActive); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/integrations?saved=email", http.StatusSeeOther)
}

// UpdatePayment stores gateway credentials only — nothing processes payments yet (see appointment.Store.SetFee).
func (h *Handlers) UpdatePayment(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	provider := r.PostForm.Get("provider")
	config := map[string]string{
		"key_id":     r.PostForm.Get("key_id"),
		"key_secret": r.PostForm.Get("key_secret"),
	}
	isActive := r.PostForm.Get("is_active") == "on"
	if err := h.Store.Upsert(r.Context(), ModulePayment, provider, config, isActive); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/admin/integrations?saved=payment", http.StatusSeeOther)
}
