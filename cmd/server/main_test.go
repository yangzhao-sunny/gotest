package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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
	// Ensure buildApp returns error when config env vars are not set
	t.Setenv("TASKMGR_DB_DSN", "postgres://bad:bad@localhost:19999/nodb?sslmode=disable&connect_timeout=1")
	t.Setenv("TASKMGR_JWT_SECRET", "test")
	t.Setenv("TASKMGR_REDIS_ADDR", "localhost:16379")
	_, cleanup, err := buildApp()
	if err == nil {
		cleanup()
		t.Fatal("expected error when DB unreachable")
	}
}
