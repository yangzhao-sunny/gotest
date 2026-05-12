//go:build integration

package user

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/test/taskmgr/internal/auth"
	"github.com/test/taskmgr/internal/middleware"
	"github.com/test/taskmgr/internal/testhelper"
)

func newTestRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Auth("test-secret"))
	v1 := r.Group("/v1")
	h.RegisterRoutes(v1)
	return r
}

func makeJWT(t *testing.T, userID string) string {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := tok.SignedString([]byte("test-secret"))
	require.NoError(t, err)
	return s
}

func Test_GetMe_Integration(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewPool(t, ctx)

	// Register a user via auth.Service
	authRepo := auth.NewRepo(pool)
	authSvc := auth.NewService(authRepo, "test-secret", 60, 7)
	u, err := authSvc.Register(ctx, "userme@example.com", "password123", "Test User")
	require.NoError(t, err)

	repo := NewRepo(pool)
	h := NewHandler(repo)
	r := newTestRouter(h)

	token := makeJWT(t, u.ID)

	// GET /v1/users/me → 200
	req := httptest.NewRequest(http.MethodGet, "/v1/users/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, u.ID, body["id"])
	assert.Equal(t, "userme@example.com", body["email"])
	assert.Equal(t, "Test User", body["display_name"])
}

func Test_PatchMe_Integration(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewPool(t, ctx)

	authRepo := auth.NewRepo(pool)
	authSvc := auth.NewService(authRepo, "test-secret", 60, 7)
	u, err := authSvc.Register(ctx, "patchme@example.com", "password123", "Original Name")
	require.NoError(t, err)

	repo := NewRepo(pool)
	h := NewHandler(repo)
	r := newTestRouter(h)

	token := makeJWT(t, u.ID)

	body := bytes.NewBufferString(`{"display_name":"Updated Name"}`)
	req := httptest.NewRequest(http.MethodPatch, "/v1/users/me", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Updated Name", resp["display_name"])
}

func Test_NoToken_Returns401(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewPool(t, ctx)

	repo := NewRepo(pool)
	h := NewHandler(repo)
	r := newTestRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/v1/users/me", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
