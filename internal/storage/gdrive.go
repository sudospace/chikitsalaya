package storage

import (
	"context"
	"io"
)

// GoogleDriveBackend is scaffolded, not implemented — see ErrNotImplemented.
// Config (OAuth client id/secret or service-account JSON, folder id) is
// real and saved via the integrations store; no network calls happen yet.
type GoogleDriveBackend struct {
	Config map[string]string
}

func NewGoogleDriveBackend(config map[string]string) *GoogleDriveBackend {
	return &GoogleDriveBackend{Config: config}
}

func (b *GoogleDriveBackend) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	return ErrNotImplemented
}

func (b *GoogleDriveBackend) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return nil, ErrNotImplemented
}

func (b *GoogleDriveBackend) Delete(ctx context.Context, key string) error {
	return ErrNotImplemented
}
