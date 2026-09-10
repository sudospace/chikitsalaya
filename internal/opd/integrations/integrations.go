// Package integrations stores the clinic's SMS/email/payment config as a
// flexible key/value bag per module, since fields differ by provider.
package integrations

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Module string

const (
	ModuleSMS     Module = "sms"
	ModuleEmail   Module = "email"
	ModulePayment Module = "payment"
)

type Integration struct {
	ID        int64
	Module    Module
	Provider  string
	Config    map[string]string
	IsActive  bool
	UpdatedAt time.Time
}

type Store struct {
	Pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool}
}

// Get returns a zero-value Integration (Provider "none") if nothing is saved yet.
func (s *Store) Get(ctx context.Context, module Module) (*Integration, error) {
	var in Integration
	var raw []byte
	err := s.Pool.QueryRow(ctx,
		`SELECT id, module, provider, config, is_active, updated_at
		 FROM integrations WHERE module = $1`,
		module).Scan(&in.ID, &in.Module, &in.Provider, &raw, &in.IsActive, &in.UpdatedAt)
	if err != nil {
		if err == pgx.ErrNoRows {
			return &Integration{Module: module, Provider: "none", Config: map[string]string{}}, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(raw, &in.Config); err != nil {
		return nil, err
	}
	return &in, nil
}

// Upsert saves the clinic's config for one module, replacing whatever was there (no history/versioning).
func (s *Store) Upsert(ctx context.Context, module Module, provider string, config map[string]string, isActive bool) error {
	raw, err := json.Marshal(config)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO integrations (module, provider, config, is_active, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (module) DO UPDATE SET
			provider = EXCLUDED.provider, config = EXCLUDED.config,
			is_active = EXCLUDED.is_active, updated_at = now()`,
		module, provider, raw, isActive)
	return err
}
