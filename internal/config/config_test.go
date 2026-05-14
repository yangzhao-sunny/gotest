package config

import (
	"os"
	"testing"
)

func TestLoad_RequiredMissing(t *testing.T) {
	os.Unsetenv("TASKMGR_DB_DSN")
	os.Unsetenv("TASKMGR_JWT_SECRET")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when required vars missing")
	}
}

func TestLoad_Defaults(t *testing.T) {
	os.Setenv("TASKMGR_DB_DSN", "postgres://x")
	os.Setenv("TASKMGR_JWT_SECRET", "s3cr3t")
	defer os.Unsetenv("TASKMGR_DB_DSN")
	defer os.Unsetenv("TASKMGR_JWT_SECRET")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DBDsn != "postgres://x" {
		t.Fatalf("DBDsn: got %q", cfg.DBDsn)
	}
	if cfg.ServerPort != "8080" {
		t.Fatalf("default ServerPort: got %q", cfg.ServerPort)
	}
	if cfg.RedisAddr != "localhost:6379" {
		t.Fatalf("default RedisAddr: got %q", cfg.RedisAddr)
	}
	if cfg.AccessTokenTTL != 60 {
		t.Fatalf("default AccessTokenTTL: got %d", cfg.AccessTokenTTL)
	}
	if cfg.RefreshTokenTTL != 30 {
		t.Fatalf("default RefreshTokenTTL: got %d", cfg.RefreshTokenTTL)
	}
	if !cfg.OtelEnabled {
		t.Fatal("OtelEnabled should default true")
	}
}

func TestLoad_OtelDisabled(t *testing.T) {
	os.Setenv("TASKMGR_DB_DSN", "postgres://x")
	os.Setenv("TASKMGR_JWT_SECRET", "s3cr3t")
	os.Setenv("TASKMGR_OTEL_ENABLED", "false")
	defer os.Unsetenv("TASKMGR_DB_DSN")
	defer os.Unsetenv("TASKMGR_JWT_SECRET")
	defer os.Unsetenv("TASKMGR_OTEL_ENABLED")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OtelEnabled {
		t.Fatal("OtelEnabled should be false")
	}
}
