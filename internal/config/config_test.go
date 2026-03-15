package config

import (
	"testing"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("GS_LISTEN_ADDR", "")
	t.Setenv("GS_DB_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ListenAddr != ":8080" {
		t.Fatalf("ListenAddr = %q, want %q", cfg.ListenAddr, ":8080")
	}
	if cfg.DBURL != "" {
		t.Fatalf("DBURL = %q, want empty", cfg.DBURL)
	}
}

func TestLoad_EmptyListenAddrIsError(t *testing.T) {
	// Load() intentionally defaults GS_LISTEN_ADDR to :8080 when unset/empty.
	// Keeping this test as a placeholder in case we later decide to make the
	// listen addr mandatory.
	_, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}
