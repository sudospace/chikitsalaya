// Package web holds shared HTTP helpers: template rendering, layout data.
package web

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"strings"
)

// Chrome is the header/branding info the "base" shell needs, resolved
// fresh per request via the func set with SetChromeResolver. Pages using
// the "auth" shell (login/2FA) never see this.
type Chrome struct {
	ClinicName       string
	Role             string
	IsAdmin          bool // gates the admin-only "Catalog"/"Setup" nav groups
	IsActingAsDoctor bool // true while an admin is viewing their linked practitioner's dashboard
	UserEmail        string
	UserInitial      string // precomputed for the avatar badge; templates have no slicing helper
	RoleLabel        string // e.g. "Doctor", or "Admin + Doctor" while acting
	CanBilling       bool   // gates the "Billing" nav link
}

type ChromeResolver func(r *http.Request) Chrome

type pageEntry struct {
	tmpl     *template.Template
	authPage bool
}

// Renderer parses each page together with the shared partials, then
// executes just that page's "title"/"content" blocks and wraps them in
// one of two shells at request time — "base" (full app chrome) for
// everything, or "auth" (bare) for pages under web/templates/auth/pages.
//
// Partials are kept in a standalone template set too, so htmx endpoints
// can render a fragment with no shell. A third set, "standalone" pages
// (web/templates/<module>/standalone/*.html), are self-contained HTML
// documents with no app chrome at all, e.g. a printable page.
type Renderer struct {
	pages      map[string]pageEntry
	partials   *template.Template
	standalone map[string]*template.Template
	baseShell  *template.Template
	authShell  *template.Template
	resolve    ChromeResolver
}

func NewRenderer() (*Renderer, error) {
	partialFiles, err := filepath.Glob(filepath.Join("web", "templates", "*", "partials", "*.html"))
	if err != nil {
		return nil, err
	}
	pageFiles, err := filepath.Glob(filepath.Join("web", "templates", "*", "pages", "*.html"))
	if err != nil {
		return nil, err
	}
	standaloneFiles, err := filepath.Glob(filepath.Join("web", "templates", "*", "standalone", "*.html"))
	if err != nil {
		return nil, err
	}

	pages := make(map[string]pageEntry, len(pageFiles))
	for _, pf := range pageFiles {
		files := append([]string{pf}, partialFiles...)
		tmpl, err := template.New(filepath.Base(pf)).ParseFiles(files...)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", pf, err)
		}
		pages[filepath.Base(pf)] = pageEntry{
			tmpl:     tmpl,
			authPage: strings.Contains(filepath.ToSlash(pf), "/auth/pages/"),
		}
	}

	baseShell, err := template.ParseFiles(filepath.Join("web", "templates", "layouts", "base.html"))
	if err != nil {
		return nil, fmt.Errorf("parse base shell: %w", err)
	}
	authShell, err := template.ParseFiles(filepath.Join("web", "templates", "layouts", "auth.html"))
	if err != nil {
		return nil, fmt.Errorf("parse auth shell: %w", err)
	}

	partials := template.New("partials")
	if len(partialFiles) > 0 {
		partials, err = partials.ParseFiles(partialFiles...)
		if err != nil {
			return nil, fmt.Errorf("parse partials: %w", err)
		}
	}

	standalone := make(map[string]*template.Template, len(standaloneFiles))
	for _, sf := range standaloneFiles {
		tmpl, err := template.New(filepath.Base(sf)).ParseFiles(sf)
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", sf, err)
		}
		standalone[filepath.Base(sf)] = tmpl
	}

	return &Renderer{pages: pages, partials: partials, standalone: standalone, baseShell: baseShell, authShell: authShell}, nil
}

// SetChromeResolver wires up per-request branding. Separate from NewRenderer
// because it depends on the session manager and company store, which would
// cycle if imported here (they already import internal/web).
func (r *Renderer) SetChromeResolver(resolve ChromeResolver) {
	r.resolve = resolve
}

type shellData struct {
	Title   string
	Content template.HTML
	Chrome  Chrome
}

func (r *Renderer) Render(w http.ResponseWriter, req *http.Request, page string, data any) {
	entry, ok := r.pages[page]
	if !ok {
		http.Error(w, "template not found: "+page, http.StatusInternalServerError)
		return
	}

	var titleBuf, contentBuf bytes.Buffer
	if err := entry.tmpl.ExecuteTemplate(&titleBuf, "title", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := entry.tmpl.ExecuteTemplate(&contentBuf, "content", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	sd := shellData{Title: titleBuf.String(), Content: template.HTML(contentBuf.String())}
	shell := r.baseShell
	if entry.authPage {
		shell = r.authShell
	} else if r.resolve != nil {
		sd.Chrome = r.resolve(req)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := shell.Execute(w, sd); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// RenderPartial executes a single named partial (its {{define "name"}}) with
// no surrounding shell — used for htmx fragment responses.
func (r *Renderer) RenderPartial(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := r.partials.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// RenderStandalone executes a full self-contained HTML document with no
// app chrome — used for the encounter print view and the public booking
// pages.
func (r *Renderer) RenderStandalone(w http.ResponseWriter, page string, data any) {
	tmpl, ok := r.standalone[page]
	if !ok {
		http.Error(w, "template not found: "+page, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
