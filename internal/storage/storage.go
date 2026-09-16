// Package storage provides a pluggable backend for where patient document
// files physically live, so a clinic can pick local disk, Google Drive, or
// S3 without the rest of the app caring which one is active.
package storage

import (
	"context"
	"errors"
	"io"
)

// Backend is the interface every storage implementation satisfies.
type Backend interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// ErrNotImplemented is returned by backends that are scaffolded (config UI
// exists, credentials save fine) but don't yet perform real I/O.
var ErrNotImplemented = errors.New("this storage backend is not yet implemented")
