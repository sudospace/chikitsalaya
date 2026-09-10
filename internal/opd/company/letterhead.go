package company

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// letterheadUploadDir mirrors uploadDir (logo.go) — outside web/ so it's
// never served by the generic static file server, only via LetterheadImageHandler.
const letterheadUploadDir = "data/uploads/letterheads"

// SaveLetterheadImageIfProvided is SaveLogoIfProvided's letterhead-image
// twin. A no-op if no file is attached — the field is optional regardless
// of which letterhead_mode is selected.
func (s *Store) SaveLetterheadImageIfProvided(r *http.Request, companyID int64) error {
	file, header, err := r.FormFile("letterhead_image")
	if err == http.ErrMissingFile {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read letterhead image upload: %w", err)
	}
	defer file.Close()

	if header.Size > maxLogoBytes {
		return fmt.Errorf("letterhead image is too large (max %dMB)", maxLogoBytes>>20)
	}

	sniff := make([]byte, 512)
	n, err := io.ReadFull(file, sniff)
	if err != nil && err != io.ErrUnexpectedEOF {
		return fmt.Errorf("read letterhead image: %w", err)
	}
	sniff = sniff[:n]
	contentType := http.DetectContentType(sniff)
	ext, ok := allowedLogoTypes[contentType]
	if !ok {
		return fmt.Errorf("unsupported image type %q (use PNG, JPEG, GIF, or WEBP)", contentType)
	}

	if err := os.MkdirAll(letterheadUploadDir, 0o755); err != nil {
		return fmt.Errorf("prepare upload directory: %w", err)
	}
	if matches, _ := filepath.Glob(filepath.Join(letterheadUploadDir, fmt.Sprintf("company_%d.*", companyID))); matches != nil {
		for _, m := range matches {
			os.Remove(m)
		}
	}

	filename := fmt.Sprintf("company_%d%s", companyID, ext)
	dst, err := os.Create(filepath.Join(letterheadUploadDir, filename))
	if err != nil {
		return fmt.Errorf("save letterhead image: %w", err)
	}
	defer dst.Close()

	if _, err := dst.Write(sniff); err != nil {
		return fmt.Errorf("save letterhead image: %w", err)
	}
	if _, err := io.Copy(dst, file); err != nil {
		return fmt.Errorf("save letterhead image: %w", err)
	}

	return s.SetLetterheadImagePath(r.Context(), companyID, filename)
}

// LetterheadImageHandler serves the clinic's uploaded letterhead image,
// used by the print view and the settings form preview.
func (h *Handlers) LetterheadImageHandler(w http.ResponseWriter, r *http.Request) {
	c, err := h.Store.GetClinic(r.Context())
	if err != nil || c.LetterheadImagePath == "" {
		http.NotFound(w, r)
		return
	}
	contentType, ok := extContentType[filepath.Ext(c.LetterheadImagePath)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeFile(w, r, filepath.Join(letterheadUploadDir, filepath.Base(c.LetterheadImagePath)))
}
