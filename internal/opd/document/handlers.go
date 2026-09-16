package document

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"chikitsalaya/internal/auth"
	"chikitsalaya/internal/storage"
	"chikitsalaya/internal/web"
)

// maxDocumentBytes caps a single upload at 25MB — there's no existing
// per-clinic-configurable size in this app, and 25MB comfortably covers a
// scanned lab report or a multi-page PDF without inviting abuse.
const maxDocumentBytes = 25 << 20

type Handlers struct {
	Store          *Store
	Registry       *storage.Registry
	Sessions       *scs.SessionManager
	Renderer       *web.Renderer
	MaxUploadBytes int64
}

// NewHandlers deliberately doesn't take a *patient.Store: patient already
// depends on this package (for its own Documents-card data), so taking a
// dependency back would be an import cycle. A nonexistent patient id is
// caught by the patients FK constraint on insert instead of a pre-check.
func NewHandlers(store *Store, registry *storage.Registry, sm *scs.SessionManager, renderer *web.Renderer) *Handlers {
	return &Handlers{
		Store: store, Registry: registry,
		Sessions: sm, Renderer: renderer, MaxUploadBytes: maxDocumentBytes,
	}
}

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// sanitizeFilename strips anything that isn't safe to embed in a storage key
// or a Content-Disposition header — the original name is kept for display
// (OriginalFilename), this is only used to build the on-disk/object key.
func sanitizeFilename(name string) string {
	name = nonAlnum.ReplaceAllString(name, "_")
	if name == "" {
		return "file"
	}
	if len(name) > 100 {
		name = name[len(name)-100:]
	}
	return name
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (h *Handlers) redirectWithError(w http.ResponseWriter, r *http.Request, patientID int64, msg string) {
	http.Redirect(w, r, "/patients/"+strconv.FormatInt(patientID, 10)+"?error="+url.QueryEscape(msg)+"#documents", http.StatusSeeOther)
}

// Upload accepts any file type (unlike the logo/letterhead uploads, which
// sniff-and-reject against an images-only allowlist) — a patient report can
// legitimately be a PDF, an image, or anything else a clinic hands you.
func (h *Handlers) Upload(w http.ResponseWriter, r *http.Request) {
	patientID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()

	if err := r.ParseMultipartForm(h.MaxUploadBytes + 1<<20); err != nil {
		h.redirectWithError(w, r, patientID, "Upload too large or malformed.")
		return
	}
	file, header, err := r.FormFile("file")
	if err == http.ErrMissingFile {
		h.redirectWithError(w, r, patientID, "Please choose a file to upload.")
		return
	}
	if err != nil {
		h.redirectWithError(w, r, patientID, "Couldn't read the upload: "+err.Error())
		return
	}
	defer file.Close()

	if header.Size > h.MaxUploadBytes {
		h.redirectWithError(w, r, patientID, fmt.Sprintf("File is too large (max %dMB).", h.MaxUploadBytes>>20))
		return
	}

	sniff := make([]byte, 512)
	n, err := io.ReadFull(file, sniff)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		h.redirectWithError(w, r, patientID, "Couldn't read the upload: "+err.Error())
		return
	}
	sniff = sniff[:n]

	contentType := header.Header.Get("Content-Type")
	if contentType == "" || contentType == "application/octet-stream" {
		contentType = http.DetectContentType(sniff)
	}
	fullReader := io.MultiReader(bytes.NewReader(sniff), file)

	backendName, err := h.Registry.Active(ctx)
	if err != nil {
		h.redirectWithError(w, r, patientID, "Couldn't determine the storage backend: "+err.Error())
		return
	}
	backend, err := h.Registry.Resolve(ctx, backendName)
	if err != nil {
		h.redirectWithError(w, r, patientID, "Storage isn't configured: "+err.Error())
		return
	}

	suffix, err := randomHex(8)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	key := fmt.Sprintf("patient-%d/%s-%s", patientID, suffix, sanitizeFilename(header.Filename))

	if err := backend.Put(ctx, key, fullReader, header.Size, contentType); err != nil {
		if errors.Is(err, storage.ErrNotImplemented) {
			h.redirectWithError(w, r, patientID, "This clinic's configured storage backend isn't available yet — pick Local in Setup > Integrations, or contact your admin.")
			return
		}
		h.redirectWithError(w, r, patientID, "Couldn't save the file: "+err.Error())
		return
	}

	uploadedBy := auth.CurrentUserID(h.Sessions, r)
	var uploadedByPtr *int64
	if uploadedBy > 0 {
		uploadedByPtr = &uploadedBy
	}

	if _, err := h.Store.Create(ctx, Document{
		PatientID:        patientID,
		OriginalFilename: header.Filename,
		StorageBackend:   backendName,
		StorageKey:       key,
		ContentType:      contentType,
		SizeBytes:        header.Size,
		Description:      r.PostForm.Get("description"),
		UploadedBy:       uploadedByPtr,
	}); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" { // foreign_key_violation: no such patient
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/patients/"+strconv.FormatInt(patientID, 10)+"#documents", http.StatusSeeOther)
}

func (h *Handlers) Download(w http.ResponseWriter, r *http.Request) {
	patientID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	docID, err := strconv.ParseInt(chi.URLParam(r, "docID"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	doc, err := h.Store.Get(ctx, docID)
	if err != nil || doc.PatientID != patientID {
		http.NotFound(w, r)
		return
	}

	backend, err := h.Registry.Resolve(ctx, doc.StorageBackend)
	if err != nil {
		http.Error(w, "Couldn't resolve storage for this document: "+err.Error(), http.StatusInternalServerError)
		return
	}
	rc, err := backend.Get(ctx, doc.StorageKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotImplemented) {
			http.Error(w, "This document's storage backend ("+doc.StorageBackend+") isn't available yet.", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "Couldn't read the file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", doc.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": doc.OriginalFilename}))
	io.Copy(w, rc)
}

func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	patientID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	docID, err := strconv.ParseInt(chi.URLParam(r, "docID"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	doc, err := h.Store.Get(ctx, docID)
	if err != nil || doc.PatientID != patientID {
		http.NotFound(w, r)
		return
	}

	// A backend that can't delete (e.g. not-yet-implemented gdrive/s3)
	// shouldn't block removing the reference — the DB row is the source of
	// truth for "is this attached to the patient", the underlying file
	// becoming orphaned is a lesser problem than an undeletable row.
	if backend, err := h.Registry.Resolve(ctx, doc.StorageBackend); err == nil {
		if err := backend.Delete(ctx, doc.StorageKey); err != nil && !errors.Is(err, storage.ErrNotImplemented) {
			http.Error(w, "Couldn't delete the file: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err := h.Store.Delete(ctx, docID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/patients/"+strconv.FormatInt(patientID, 10)+"#documents", http.StatusSeeOther)
}
