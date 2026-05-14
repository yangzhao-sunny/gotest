//go:build integration

package project

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
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/test/taskmgr/internal/auth"
	"github.com/test/taskmgr/internal/middleware"
	"github.com/test/taskmgr/internal/testhelper"
)

// noopTaskDeleter satisfies TaskDeleter but does nothing (no tasks in project tests).
type noopTaskDeleter struct{}

func (noopTaskDeleter) SoftDeleteByProject(_ context.Context, _ pgx.Tx, _ string) error {
	return nil
}

func newProjectRouter(h *Handler, secret string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Auth(secret))
	v1 := r.Group("/v1")
	h.RegisterRoutes(v1)
	return r
}

func makeProjectJWT(t *testing.T, userID string) string {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := tok.SignedString([]byte("test-secret"))
	require.NoError(t, err)
	return s
}

func registerUser(t *testing.T, ctx context.Context, pool interface{ QueryRow(context.Context, string, ...any) pgx.Row }, email string) string {
	t.Helper()
	// Use auth repo directly
	return ""
}

func Test_ProjectCRUD_Integration(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewPool(t, ctx)

	authRepo := auth.NewRepo(pool)
	authSvc := auth.NewService(authRepo, "test-secret", 60, 7)
	userA, err := authSvc.Register(ctx, "projuser@example.com", "password123", "User A")
	require.NoError(t, err)

	repo := NewRepo(pool)
	h := NewHandler(repo, noopTaskDeleter{}, nil)
	r := newProjectRouter(h, "test-secret")
	tokenA := makeProjectJWT(t, userA.ID)

	// Create project
	body := bytes.NewBufferString(`{"name":"My Project","description":"A desc"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/projects", body)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var created map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	projectID := created["id"].(string)
	assert.Equal(t, "My Project", created["name"])

	// List projects
	req = httptest.NewRequest(http.MethodGet, "/v1/projects", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	data := listResp["data"].([]any)
	assert.Len(t, data, 1)

	// Get project
	req = httptest.NewRequest(http.MethodGet, "/v1/projects/"+projectID, nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Patch project
	patchBody := bytes.NewBufferString(`{"name":"Updated Project"}`)
	req = httptest.NewRequest(http.MethodPatch, "/v1/projects/"+projectID, patchBody)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var updated map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	assert.Equal(t, "Updated Project", updated["name"])

	// Delete project
	req = httptest.NewRequest(http.MethodDelete, "/v1/projects/"+projectID, nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)

	// List after delete → empty
	req = httptest.NewRequest(http.MethodGet, "/v1/projects", nil)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	data = listResp["data"].([]any)
	assert.Len(t, data, 0)
}

func Test_ProjectUserBIsolation_Integration(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewPool(t, ctx)

	authRepo := auth.NewRepo(pool)
	authSvc := auth.NewService(authRepo, "test-secret", 60, 7)
	userA, err := authSvc.Register(ctx, "isoA@example.com", "password123", "User A")
	require.NoError(t, err)
	userB, err := authSvc.Register(ctx, "isoB@example.com", "password123", "User B")
	require.NoError(t, err)

	repo := NewRepo(pool)
	h := NewHandler(repo, noopTaskDeleter{}, nil)
	r := newProjectRouter(h, "test-secret")

	tokenA := makeProjectJWT(t, userA.ID)
	tokenB := makeProjectJWT(t, userB.ID)

	// User A creates project
	body := bytes.NewBufferString(`{"name":"A's Project"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/projects", body)
	req.Header.Set("Authorization", "Bearer "+tokenA)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	var created map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	projectID := created["id"].(string)

	// User B tries to get user A's project → 404
	req = httptest.NewRequest(http.MethodGet, "/v1/projects/"+projectID, nil)
	req.Header.Set("Authorization", "Bearer "+tokenB)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func Test_ProjectCursorPagination_Integration(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewPool(t, ctx)

	authRepo := auth.NewRepo(pool)
	authSvc := auth.NewService(authRepo, "test-secret", 60, 7)
	user, err := authSvc.Register(ctx, "paguser@example.com", "password123", "Pager")
	require.NoError(t, err)

	repo := NewRepo(pool)
	h := NewHandler(repo, noopTaskDeleter{}, nil)
	r := newProjectRouter(h, "test-secret")
	token := makeProjectJWT(t, user.ID)

	// Create 3 projects
	for i := 0; i < 3; i++ {
		b := bytes.NewBufferString(`{"name":"Project"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/projects", b)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusCreated, w.Code)
	}

	// List with limit=2 → should get 2 + next_cursor
	req := httptest.NewRequest(http.MethodGet, "/v1/projects?limit=2", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var page1 map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page1))
	data := page1["data"].([]any)
	assert.Len(t, data, 2)
	nextCursor, ok := page1["next_cursor"].(string)
	assert.True(t, ok && nextCursor != "", "expected next_cursor")

	// Fetch page 2
	req = httptest.NewRequest(http.MethodGet, "/v1/projects?limit=2&cursor="+nextCursor, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var page2 map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &page2))
	data2 := page2["data"].([]any)
	assert.Len(t, data2, 1)
}
