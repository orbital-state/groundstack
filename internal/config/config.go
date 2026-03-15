package config

import (
	"fmt"
	"os"
)

type Config struct {
	ListenAddr string
	DBURL      string
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddr: getenvDefault("GS_LISTEN_ADDR", ":8080"),
		DBURL:      os.Getenv("GS_DB_URL"),
	}

	if cfg.ListenAddr == "" {
		return Config{}, fmt.Errorf("GS_LISTEN_ADDR must not be empty")
	}

	// Note: GS_DB_URL is intentionally optional for the initial scaffold so
	// unit tests and direct local runs can start without Postgres.
	return cfg, nil
}

func getenvDefault(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}
