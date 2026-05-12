package config

import (
	"fmt"
	"os"
)

type Config struct {
	ServerPort      string
	DBDsn           string
	RedisAddr       string
	JWTSecret       string
	AccessTokenTTL  int // minutes
	RefreshTokenTTL int // days
	OtelEnabled     bool
	OtelEndpoint    string
}

func Load() (*Config, error) {
	dsn := os.Getenv("TASKMGR_DB_DSN")
	if dsn == "" {
		return nil, fmt.Errorf("TASKMGR_DB_DSN is required")
	}
	secret := os.Getenv("TASKMGR_JWT_SECRET")
	if secret == "" {
		return nil, fmt.Errorf("TASKMGR_JWT_SECRET is required")
	}
	return &Config{
		ServerPort:      envOr("TASKMGR_PORT", "8080"),
		DBDsn:           dsn,
		RedisAddr:       envOr("TASKMGR_REDIS_ADDR", "localhost:6379"),
		JWTSecret:       secret,
		AccessTokenTTL:  60,
		RefreshTokenTTL: 30,
		OtelEnabled:     os.Getenv("TASKMGR_OTEL_ENABLED") != "false",
		OtelEndpoint:    envOr("TASKMGR_OTEL_ENDPOINT", "localhost:4317"),
	}, nil
}

func EnvOr(key, def string) string { return envOr(key, def) }

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
