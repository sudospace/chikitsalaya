package company

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// uploadDir holds company logos on disk, outside web/ so they're never
// exposed by the generic static file server — only through LogoHandler.
const uploadDir = "data/uploads/logos"

const maxLogoBytes = 2 << 20 // 2MB

// allowedLogoTypes maps sniffed content types to a file extension. SVG is
// deliberately excluded — it can embed <script>/event handlers, an XSS risk
// for a file that gets served back to every visitor.
var allowedLogoTypes = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// SaveLogoIfProvided validates an optional "logo" upload by sniffing content
// bytes (never trusting the client-supplied filename/extension) and stores
// it as company_<id>.<ext>. A no-op if no file is attached.
func (s *Store) SaveLogoIfProvided(r *http.Request, companyID int64) error {
	file, header, err := r.FormFile("logo")
	if err == http.ErrMissingFile {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read logo upload: %w", err)
	}
	defer file.Close()

	if header.Size > maxLogoBytes {
		return fmt.Errorf("logo image is too large (max %dMB)", maxLogoBytes>>20)
	}

	sniff := make([]byte, 512)
	n, err := io.ReadFull(file, sniff)
	if err != nil && err != io.ErrUnexpectedEOF {
		return fmt.Errorf("read logo image: %w", err)
	}
	sniff = sniff[:n]
	contentType := http.DetectContentType(sniff)
	ext, ok := allowedLogoTypes[contentType]
	if !ok {
		return fmt.Errorf("unsupported image type %q (use PNG, JPEG, GIF, or WEBP)", contentType)
	}

	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return fmt.Errorf("prepare upload directory: %w", err)
	}
	// Remove any previous logo for this company under a different extension.
	if matches, _ := filepath.Glob(filepath.Join(uploadDir, fmt.Sprintf("company_%d.*", companyID))); matches != nil {
		for _, m := range matches {
			os.Remove(m)
		}
	}

	filename := fmt.Sprintf("company_%d%s", companyID, ext)
	dst, err := os.Create(filepath.Join(uploadDir, filename))
	if err != nil {
		return fmt.Errorf("save logo image: %w", err)
	}
	defer dst.Close()

	if _, err := dst.Write(sniff); err != nil {
		return fmt.Errorf("save logo image: %w", err)
	}
	if _, err := io.Copy(dst, file); err != nil {
		return fmt.Errorf("save logo image: %w", err)
	}

	return s.SetLogoPath(r.Context(), companyID, filename)
}

var extContentType = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
}

// LogoHandler serves the clinic's logo — used as the header image and,
// mounted at /favicon.ico too, the browser tab icon.
func (h *Handlers) LogoHandler(w http.ResponseWriter, r *http.Request) {
	c, err := h.Store.GetClinic(r.Context())
	if err != nil || c.LogoPath == "" {
		http.NotFound(w, r)
		return
	}
	contentType, ok := extContentType[filepath.Ext(c.LogoPath)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeFile(w, r, filepath.Join(uploadDir, filepath.Base(c.LogoPath)))
}
