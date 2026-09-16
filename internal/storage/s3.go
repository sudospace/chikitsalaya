package storage

import (
	"context"
	"io"
)

// S3Backend is scaffolded, not implemented — see ErrNotImplemented.
// Config (bucket/region/access key/secret/endpoint) is real and saved via
// the integrations store; no network calls happen yet.
type S3Backend struct {
	Config map[string]string
}

func NewS3Backend(config map[string]string) *S3Backend {
	return &S3Backend{Config: config}
}

func (b *S3Backend) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	return ErrNotImplemented
}

func (b *S3Backend) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return nil, ErrNotImplemented
}

func (b *S3Backend) Delete(ctx context.Context, key string) error {
	return ErrNotImplemented
}
