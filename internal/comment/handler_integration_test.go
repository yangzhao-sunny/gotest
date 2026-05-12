//go:build integration

package comment

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
	"github.com/test/taskmgr/internal/project"
	"github.com/test/taskmgr/internal/task"
	"github.com/test/taskmgr/internal/testhelper"
)

func newCommentRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Auth("test-secret"))
	v1 := r.Group("/v1")
	h.RegisterRoutes(v1)
	return r
}

func makeCommentJWT(t *testing.T, userID string) string {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := tok.SignedString([]byte("test-secret"))
	require.NoError(t, err)
	return s
}

func Test_CommentRateLimit_Integration(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewPool(t, ctx)
	redis := testhelper.NewRedis(t, ctx)

	// Setup: user, project, task
	authRepo := auth.NewRepo(pool)
	authSvc := auth.NewService(authRepo, "test-secret", 60, 7)
	u, err := authSvc.Register(ctx, "commentuser@example.com", "password123", "Comment User")
	require.NoError(t, err)

	projRepo := project.NewRepo(pool)
	p, err := projRepo.Create(ctx, u.ID, "Comment Project", nil)
	require.NoError(t, err)

	taskRepo := task.NewRepo(pool)
	tk, err := taskRepo.Create(ctx, p.ID, "Comment Task", 0, nil, nil)
	require.NoError(t, err)

	repo := NewRepo(pool)
	h := NewHandler(repo, redis)
	r := newCommentRouter(h)
	token := makeCommentJWT(t, u.ID)

	post := func(body string) *httptest.ResponseRecorder {
		b := bytes.NewBufferString(`{"body":"` + body + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/tasks/"+tk.ID+"/comments", b)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	// Post 3 comments → all 201
	for i := 0; i < 3; i++ {
		w := post("Comment body")
		assert.Equal(t, http.StatusCreated, w.Code, "comment %d should be 201", i+1)
	}

	// 4th → 429
	w := post("One too many")
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	var errResp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	errObj := errResp["error"].(map[string]any)
	assert.Equal(t, "rate_limited", errObj["code"])
}

func Test_CommentList_Integration(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewPool(t, ctx)
	redis := testhelper.NewRedis(t, ctx)

	authRepo := auth.NewRepo(pool)
	authSvc := auth.NewService(authRepo, "test-secret", 60, 7)
	u, err := authSvc.Register(ctx, "listcomment@example.com", "password123", "List Comment User")
	require.NoError(t, err)

	projRepo := project.NewRepo(pool)
	p, err := projRepo.Create(ctx, u.ID, "List Comment Project", nil)
	require.NoError(t, err)

	taskRepo := task.NewRepo(pool)
	tk, err := taskRepo.Create(ctx, p.ID, "List Comment Task", 0, nil, nil)
	require.NoError(t, err)

	// Insert comments directly via repo
	repo := NewRepo(pool)
	for i := 0; i < 3; i++ {
		_, err := repo.Create(ctx, tk.ID, u.ID, "body")
		require.NoError(t, err)
	}

	h := NewHandler(repo, redis)
	r := newCommentRouter(h)
	token := makeCommentJWT(t, u.ID)

	// List with limit=2 → 2 results + cursor
	req := httptest.NewRequest(http.MethodGet, "/v1/tasks/"+tk.ID+"/comments?limit=2", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].([]any)
	assert.Len(t, data, 2)
	nextCursor, ok := resp["next_cursor"].(string)
	assert.True(t, ok && nextCursor != "", "expected next_cursor")

	// Fetch page 2
	req = httptest.NewRequest(http.MethodGet, "/v1/tasks/"+tk.ID+"/comments?limit=2&cursor="+nextCursor, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data2 := resp["data"].([]any)
	assert.Len(t, data2, 1)
}
