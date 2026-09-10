// Package config loads app configuration from environment variables via koanf.
package config

import (
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/v2"
)

type Config struct {
	Port        string
	DatabaseURL string
}

func Load() Config {
	k := koanf.New(".")
	// Env vars load as-is (no prefix/delimiter mangling needed for our two flat keys).
	_ = k.Load(env.Provider(".", env.Opt{}), nil)

	cfg := Config{
		Port:        k.String("PORT"),
		DatabaseURL: k.String("DATABASE_URL"),
	}
	if cfg.Port == "" {
		cfg.Port = "8090"
	}
	if cfg.DatabaseURL == "" {
		cfg.DatabaseURL = "postgres://chikitsalaya:chikitsalaya@localhost:5442/chikitsalaya?sslmode=disable"
	}
	return cfg
}
