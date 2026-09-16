package storage

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalEncryptedBackend stores files AES-256-GCM-encrypted at rest under
// Root. The key never lives in the DB (unlike the SMS/email/payment
// secrets, which do) — it comes from an env var so a database dump alone
// can't decrypt stored files.
type LocalEncryptedBackend struct {
	Root string
	Key  [32]byte
}

// NewLocalEncryptedBackend validates hexKey is exactly 64 hex chars (32
// bytes). Returns an error (never panics) on a bad key — the caller is
// responsible for treating that as fatal at boot when local is the active
// backend, matching this app's existing fail-fast convention.
func NewLocalEncryptedBackend(root, hexKey string) (*LocalEncryptedBackend, error) {
	raw, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("local storage key: not valid hex: %w", err)
	}
	if len(raw) != 32 {
		return nil, fmt.Errorf("local storage key: must decode to 32 bytes (64 hex chars), got %d", len(raw))
	}
	b := &LocalEncryptedBackend{Root: root}
	copy(b.Key[:], raw)
	return b, nil
}

// resolvePath guards against a key escaping Root via ".." or an absolute path.
func (b *LocalEncryptedBackend) resolvePath(key string) (string, error) {
	if key == "" || strings.Contains(key, "..") || filepath.IsAbs(key) {
		return "", errors.New("invalid storage key")
	}
	return filepath.Join(b.Root, key), nil
}

func (b *LocalEncryptedBackend) Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	path, err := b.resolvePath(key)
	if err != nil {
		return err
	}
	plaintext, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read upload: %w", err)
	}

	block, err := aes.NewCipher(b.Key[:])
	if err != nil {
		return fmt.Errorf("init cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("init gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("prepare storage directory: %w", err)
	}
	if err := os.WriteFile(path, ciphertext, 0o600); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func (b *LocalEncryptedBackend) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	path, err := b.resolvePath(key)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	block, err := aes.NewCipher(b.Key[:])
	if err != nil {
		return nil, fmt.Errorf("init cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("init gcm: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return nil, errors.New("corrupt stored file: too short")
	}
	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt file: %w", err)
	}
	return io.NopCloser(bytes.NewReader(plaintext)), nil
}

func (b *LocalEncryptedBackend) Delete(ctx context.Context, key string) error {
	path, err := b.resolvePath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete file: %w", err)
	}
	return nil
}
