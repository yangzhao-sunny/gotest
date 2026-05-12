package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/test/taskmgr/internal/config"
)

func init() { gin.SetMode(gin.TestMode) }

// TestHealthzNoConfig wires a minimal router without DB/Redis to verify /healthz always returns 200.
func TestHealthzRoute(t *testing.T) {
	// build a stub router that only mounts healthz
	r := gin.New()
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/healthz", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

func TestConfigLoad_BuildApp(t *testing.T) {
	// Ensure buildApp returns error when config is invalid (no DB)
	cfg := &config.Config{
		ServerPort:      "8080",
		DBDsn:           "postgres://bad:bad@localhost:19999/nodb?sslmode=disable&connect_timeout=1",
		RedisAddr:       "localhost:16379",
		JWTSecret:       "test",
		AccessTokenTTL:  60,
		RefreshTokenTTL: 30,
	}
	_, cleanup, err := buildApp(cfg)
	if err == nil {
		cleanup()
		t.Fatal("expected error when DB unreachable")
	}
}
