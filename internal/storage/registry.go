package storage

import (
	"context"
	"fmt"

	"chikitsalaya/internal/opd/integrations"
)

// Registry resolves the right Backend for a document PER ROW (via its own
// stored storage_backend value), not "whatever the clinic's active choice
// currently is" — so switching the active backend later doesn't strand
// previously-uploaded files under the old one.
type Registry struct {
	Integrations *integrations.Store
	LocalRoot    string
	LocalKey     string // hex-encoded, from config; empty if not configured
}

// Resolve returns the Backend for a specific already-known backend name
// (read from a patient_documents row) — used for Get/Delete on an existing
// document. Takes a context because the gdrive/s3 paths need to fetch that
// module's saved config from the integrations store.
func (r *Registry) Resolve(ctx context.Context, backendName string) (Backend, error) {
	switch backendName {
	case "local":
		if r.LocalKey == "" {
			return nil, fmt.Errorf("local storage backend has no encryption key configured")
		}
		return NewLocalEncryptedBackend(r.LocalRoot, r.LocalKey)
	case "gdrive":
		in, err := r.Integrations.Get(ctx, integrations.ModuleStorage)
		if err != nil {
			return nil, err
		}
		return NewGoogleDriveBackend(in.Config), nil
	case "s3":
		in, err := r.Integrations.Get(ctx, integrations.ModuleStorage)
		if err != nil {
			return nil, err
		}
		return NewS3Backend(in.Config), nil
	default:
		return nil, fmt.Errorf("unknown storage backend %q", backendName)
	}
}

// Active returns the clinic's currently-configured choice (reads the
// "storage" module row from Integrations) — used when deciding which
// backend a NEW upload should go to. Falls back to "local" if unset,
// mirroring integrations.Store.Get's own "none" zero-value convention.
func (r *Registry) Active(ctx context.Context) (string, error) {
	in, err := r.Integrations.Get(ctx, integrations.ModuleStorage)
	if err != nil {
		return "", err
	}
	if in.Provider == "" || in.Provider == "none" {
		return "local", nil
	}
	return in.Provider, nil
}
