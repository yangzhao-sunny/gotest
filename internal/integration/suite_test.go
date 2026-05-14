//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/test/taskmgr/internal/auth"
	"github.com/test/taskmgr/internal/comment"
	"github.com/test/taskmgr/internal/config"
	"github.com/test/taskmgr/internal/health"
	"github.com/test/taskmgr/internal/middleware"
	"github.com/test/taskmgr/internal/notify"
	"github.com/test/taskmgr/internal/project"
	"github.com/test/taskmgr/internal/stats"
	"github.com/test/taskmgr/internal/task"
	"github.com/test/taskmgr/internal/testhelper"
	"github.com/test/taskmgr/internal/user"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
)

func init() { gin.SetMode(gin.TestMode) }

type testServer struct {
	srv    *httptest.Server
	pool   *pgxpool.Pool
	redis  *goredis.Client
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	ctx := context.Background()
	pool := testhelper.NewPool(t, ctx)
	redisClient := testhelper.NewRedis(t, ctx)

	cfg := &config.Config{JWTSecret: "test-secret", AccessTokenTTL: 60, RefreshTokenTTL: 30}

	notifier := notify.New(256)
	notifyCtx, notifyCancel := context.WithCancel(ctx)
	go notifier.Run(notifyCtx)
	t.Cleanup(notifyCancel)

	authRepo := auth.NewRepo(pool)
	userRepo := user.NewRepo(pool)
	projRepo := project.NewRepo(pool)
	taskRepo := task.NewRepo(pool)
	commentRepo := comment.NewRepo(pool)
	statsRepo := stats.NewRepo(pool)

	authSvc := auth.NewService(authRepo, cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)

	authH := auth.NewHandler(authSvc)
	userH := user.NewHandler(userRepo)
	projH := project.NewHandler(projRepo, taskRepo, redisClient)
	taskH := task.NewHandler(taskRepo, projRepo, notifier)
	commentH := comment.NewHandler(commentRepo, redisClient)
	statsH := stats.NewHandler(statsRepo, pool, redisClient)
	healthH := health.NewHandler(pool, redisClient)

	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Logger())
	r.GET("/healthz", healthH.Healthz)
	r.GET("/readyz", healthH.Readyz)

	v1 := r.Group("/v1")
	v1.POST("/auth/register", authH.Register)
	v1.POST("/auth/login", authH.Login)
	v1.POST("/auth/refresh", authH.Refresh)

	protected := v1.Group("", middleware.Auth(cfg.JWTSecret))
	protected.POST("/auth/logout", authH.Logout)
	userH.RegisterRoutes(protected)
	projH.RegisterRoutes(protected)
	taskH.RegisterRoutes(protected)
	commentH.RegisterRoutes(protected)
	statsH.RegisterRoutes(protected)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	return &testServer{srv: srv, pool: pool, redis: redisClient}
}

// helpers

func (s *testServer) do(t *testing.T, method, path string, body any, token string) *http.Response {
	t.Helper()
	var buf *bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewBuffer(b)
	} else {
		buf = &bytes.Buffer{}
	}
	req, err := http.NewRequest(method, s.srv.URL+path, buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func decode(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatal(err)
	}
}

func (s *testServer) register(t *testing.T, email, password, name string) string {
	t.Helper()
	resp := s.do(t, "POST", "/v1/auth/register", map[string]string{
		"email": email, "password": password, "display_name": name,
	}, "")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: want 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()
	return s.login(t, email, password)
}

func (s *testServer) login(t *testing.T, email, password string) string {
	t.Helper()
	resp := s.do(t, "POST", "/v1/auth/login", map[string]string{
		"email": email, "password": password,
	}, "")
	var body map[string]string
	decode(t, resp, &body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: want 200, got %d", resp.StatusCode)
	}
	return body["access_token"]
}

func jsonStr(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func bodyStr(resp *http.Response) string {
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	resp.Body.Close()
	return buf.String()
}

func mustContain(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Fatalf("expected %q to contain %q", s, sub)
	}
}

func assertStatus(t *testing.T, resp *http.Response, want int) string {
	t.Helper()
	body := bodyStr(resp)
	if resp.StatusCode != want {
		t.Fatalf("want %d, got %d: %s", want, resp.StatusCode, body)
	}
	return body
}

// unused import guard
var _ = fmt.Sprintf
var _ = time.Now
var _ = jsonStr
