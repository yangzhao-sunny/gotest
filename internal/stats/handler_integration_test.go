//go:build integration

package stats

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"encoding/json"

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

func newStatsRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Auth("test-secret"))
	v1 := r.Group("/v1")
	h.RegisterRoutes(v1)
	return r
}

func makeStatsJWT(t *testing.T, userID string) string {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := tok.SignedString([]byte("test-secret"))
	require.NoError(t, err)
	return s
}

func Test_Stats_Integration(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewPool(t, ctx)
	redis := testhelper.NewRedis(t, ctx)

	authRepo := auth.NewRepo(pool)
	authSvc := auth.NewService(authRepo, "test-secret", 60, 7)
	u, err := authSvc.Register(ctx, "statsuser@example.com", "password123", "Stats User")
	require.NoError(t, err)

	projRepo := project.NewRepo(pool)
	p, err := projRepo.Create(ctx, u.ID, "Stats Project", nil)
	require.NoError(t, err)

	taskRepo := task.NewRepo(pool)

	// Create tasks in various statuses
	todoTask, err := taskRepo.Create(ctx, p.ID, "Todo Task", 0, nil, nil)
	require.NoError(t, err)
	_ = todoTask

	doingTask, err := taskRepo.Create(ctx, p.ID, "Doing Task", 0, nil, nil)
	require.NoError(t, err)
	// Transition to doing
	doingStatus := task.StatusDoing
	_, err = taskRepo.Update(ctx, doingTask.ID, task.UpdateFields{Status: &doingStatus})
	require.NoError(t, err)

	doneTask, err := taskRepo.Create(ctx, p.ID, "Done Task", 0, nil, nil)
	require.NoError(t, err)
	// Transition done task: todo → doing → done
	_, err = taskRepo.Update(ctx, doneTask.ID, task.UpdateFields{Status: &doingStatus})
	require.NoError(t, err)
	doneStatus := task.StatusDone
	_, err = taskRepo.Update(ctx, doneTask.ID, task.UpdateFields{Status: &doneStatus})
	require.NoError(t, err)

	archivedTask, err := taskRepo.Create(ctx, p.ID, "Archived Task", 0, nil, nil)
	require.NoError(t, err)
	// Transition archived: todo → doing → done → archived
	_, err = taskRepo.Update(ctx, archivedTask.ID, task.UpdateFields{Status: &doingStatus})
	require.NoError(t, err)
	_, err = taskRepo.Update(ctx, archivedTask.ID, task.UpdateFields{Status: &doneStatus})
	require.NoError(t, err)
	archivedStatus := task.StatusArchived
	_, err = taskRepo.Update(ctx, archivedTask.ID, task.UpdateFields{Status: &archivedStatus})
	require.NoError(t, err)

	// Create overdue task: doing + past due_date
	pastDate := "2020-01-01"
	overdueTask, err := taskRepo.Create(ctx, p.ID, "Overdue Task", 0, nil, &pastDate)
	require.NoError(t, err)
	_, err = taskRepo.Update(ctx, overdueTask.ID, task.UpdateFields{Status: &doingStatus})
	require.NoError(t, err)

	repo := NewRepo(pool)
	h := NewHandler(repo, pool, redis)
	r := newStatsRouter(h)
	token := makeStatsJWT(t, u.ID)

	// GET /v1/projects/:id/stats
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/"+p.ID+"/stats", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var s Stats
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &s))
	assert.Equal(t, 1, s.Todo, "todo count")
	assert.Equal(t, 2, s.Doing, "doing count (doingTask + overdueTask)")
	assert.Equal(t, 1, s.Done, "done count")
	assert.Equal(t, 1, s.Archived, "archived count")
	assert.Equal(t, 1, s.Overdue, "overdue count (overdueTask is doing with past due date)")

	// Verify Redis key exists after first call
	redisKey := "stats:" + p.ID
	val, err := redis.Get(ctx, redisKey).Result()
	require.NoError(t, err)
	assert.NotEmpty(t, val)

	// Second call should hit cache
	req = httptest.NewRequest(http.MethodGet, "/v1/projects/"+p.ID+"/stats", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var s2 Stats
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &s2))
	assert.Equal(t, s, s2)
}
