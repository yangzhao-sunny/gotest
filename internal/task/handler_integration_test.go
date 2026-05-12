//go:build integration

package task

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
	"github.com/test/taskmgr/internal/testhelper"
)

// noopNotifier satisfies Notifier.
type noopNotifier struct{}

func (noopNotifier) Publish(_, _ string) {}

func newTaskRouter(taskRepo *Repo, projRepo *project.Repo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Auth("test-secret"))
	v1 := r.Group("/v1")
	h := NewHandler(taskRepo, projRepo, noopNotifier{})
	h.RegisterRoutes(v1)
	return r
}

func makeTaskJWT(t *testing.T, userID string) string {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := tok.SignedString([]byte("test-secret"))
	require.NoError(t, err)
	return s
}

func setupTaskTest(t *testing.T) (context.Context, *Repo, *project.Repo, string, string, string) {
	t.Helper()
	ctx := context.Background()
	pool := testhelper.NewPool(t, ctx)

	authRepo := auth.NewRepo(pool)
	authSvc := auth.NewService(authRepo, "test-secret", 60, 7)
	u, err := authSvc.Register(ctx, "taskuser@example.com", "password123", "Task User")
	require.NoError(t, err)

	projRepo := project.NewRepo(pool)
	p, err := projRepo.Create(ctx, u.ID, "Test Project", nil)
	require.NoError(t, err)

	taskRepo := NewRepo(pool)
	token := makeTaskJWT(t, u.ID)
	return ctx, taskRepo, projRepo, u.ID, p.ID, token
}

func Test_TaskCRUD_Integration(t *testing.T) {
	_, taskRepo, projRepo, _, projectID, token := setupTaskTest(t)
	r := newTaskRouter(taskRepo, projRepo)

	// Create task
	body := bytes.NewBufferString(`{"title":"My Task","priority":1}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/"+projectID+"/tasks", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	var created map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	taskID := created["id"].(string)
	assert.Equal(t, "My Task", created["title"])
	assert.Equal(t, "todo", created["status"])

	// List tasks → 1 result
	req = httptest.NewRequest(http.MethodGet, "/v1/projects/"+projectID+"/tasks", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var listResp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	data := listResp["data"].([]any)
	assert.Len(t, data, 1)

	// SoftDelete task
	req = httptest.NewRequest(http.MethodDelete, "/v1/tasks/"+taskID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code)

	// List → 0 results
	req = httptest.NewRequest(http.MethodGet, "/v1/projects/"+projectID+"/tasks", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	data = listResp["data"].([]any)
	assert.Len(t, data, 0)
}

func Test_TaskTransitions_Integration(t *testing.T) {
	_, taskRepo, projRepo, _, projectID, token := setupTaskTest(t)
	r := newTaskRouter(taskRepo, projRepo)

	// Create task
	body := bytes.NewBufferString(`{"title":"Transition Task"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/"+projectID+"/tasks", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	var created map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	taskID := created["id"].(string)

	transition := func(to string) *httptest.ResponseRecorder {
		b := bytes.NewBufferString(`{"status":"` + to + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/tasks/"+taskID+"/transitions", b)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	// todo → doing
	w = transition("doing")
	assert.Equal(t, http.StatusOK, w.Code)

	// doing → done
	w = transition("done")
	assert.Equal(t, http.StatusOK, w.Code)

	// done → doing (reopen)
	w = transition("doing")
	assert.Equal(t, http.StatusOK, w.Code)
}

func Test_InvalidTransition_Returns409(t *testing.T) {
	_, taskRepo, projRepo, _, projectID, token := setupTaskTest(t)
	r := newTaskRouter(taskRepo, projRepo)

	// Create task (starts as todo)
	body := bytes.NewBufferString(`{"title":"Bad Transition Task"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/projects/"+projectID+"/tasks", body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)
	var created map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	taskID := created["id"].(string)

	// todo → archived (invalid)
	b := bytes.NewBufferString(`{"status":"archived"}`)
	req = httptest.NewRequest(http.MethodPost, "/v1/tasks/"+taskID+"/transitions", b)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusConflict, w.Code)

	var errResp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &errResp))
	errObj := errResp["error"].(map[string]any)
	assert.Equal(t, "task_invalid_transition", errObj["code"])
}
