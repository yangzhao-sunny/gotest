# Task Manager Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` or `gkit:executing-plans` to implement this plan task-by-task. Any schema migration was already authored, applied, and committed by `sql-migration` before you ran — do not re-author it.

**Goal:** Deliver a production-grade multi-user task management HTTP/JSON service with JWT auth, cursor pagination, Redis caching/rate-limiting, state machine, and OpenTelemetry observability.

**Architecture:** Seven internal domain packages (auth, user, project, task, comment, stats, health) wired by hand in `cmd/server/buildApp`. All CRUD uses pgx v5 direct SQL; stats aggregation may use GORM. Redis serves two roles: 60 s stats cache and per-user/per-task comment rate limiting via INCR+EXPIRE.

**Tech Stack:** Gin, hand-written DI, pgx v5, GORM (stats only), golang-migrate, oapi-codegen, slog JSON, OpenTelemetry OTLP, testcontainers-go, Redis 7, Postgres 16

---

## Tasks

1. **Config** — load all `TASKMGR_*` env vars into a typed struct with defaults
2. **Database** — pgx pool setup + migration version check on startup
3. **Redis** — Redis client setup with ping readiness check
4. **Migrations** — SQL schema for all five tables + migrate subcommand
5. **Middleware** — request_id injection, structured logging, auth JWT middleware
6. **Auth domain** — register, login, refresh, logout handlers + repo
7. **User domain** — GET/PATCH /v1/users/me
8. **Project domain** — CRUD with cursor pagination and soft delete
9. **Task domain** — CRUD + state machine transitions + cursor pagination
10. **Comment domain** — post/list with Redis rate limiting
11. **Stats domain** — aggregation query + Redis 60 s cache
12. **Health domain** — /healthz and /readyz probes
13. **Notify** — async assignee-change event channel + consumer goroutine
14. **Wire** — assemble buildApp, graceful shutdown (30 s SIGTERM)
15. **oapi-codegen** — generate server stubs from api/openapi.yaml; ensure handlers satisfy interface
16. **Integration tests** — testcontainers-go suite covering all 10 acceptance scenarios
17. **Migrate subcommand** — `cmd/migrate/main.go` with `up|down` subcommands
18. **Docker Compose** — wire service into docker-compose.yml with health dependencies
19. **README** — local start, test, migrate, client-gen instructions

---

### Task 1: Config

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write failing test**
  ```go
  // internal/config/config_test.go
  package config_test

  import (
      "os"
      "testing"
      "github.com/test/taskmgr/internal/config"
  )

  func TestLoad(t *testing.T) {
      os.Setenv("TASKMGR_DB_DSN", "postgres://x")
      os.Setenv("TASKMGR_REDIS_ADDR", "localhost:6379")
      os.Setenv("TASKMGR_JWT_SECRET", "s3cr3t")
      cfg, err := config.Load()
      if err != nil { t.Fatal(err) }
      if cfg.DBDsn != "postgres://x" { t.Fatalf("got %q", cfg.DBDsn) }
      if cfg.ServerPort != "8080" { t.Fatalf("default port: got %q", cfg.ServerPort) }
  }
  ```
- [ ] **Step 2: Run to verify failure**
  Run: `go test ./internal/config/... -v`
  Expected: FAIL (package not found)
- [ ] **Step 3: Implement**
  ```go
  // internal/config/config.go
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
      AccessTokenTTL  int // minutes, default 60
      RefreshTokenTTL int // days, default 30
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

  func envOr(key, def string) string {
      if v := os.Getenv(key); v != "" { return v }
      return def
  }
  ```
- [ ] **Step 4: Run to verify pass**
  Run: `go test ./internal/config/... -v`
  Expected: PASS
- [ ] **Step 5: Commit**
  `git add internal/config/ && git commit -m "feat(config): typed env-var config loader"`

---

### Task 2: Database

**Files:**
- Create: `internal/db/db.go`
- Test: `internal/db/db_test.go` (testcontainers-go)

- [ ] **Step 1: Write failing test**
  ```go
  // internal/db/db_test.go
  package db_test

  import (
      "context"
      "testing"
      "github.com/test/taskmgr/internal/db"
      "github.com/testcontainers/testcontainers-go/modules/postgres"
  )

  func TestNewPool(t *testing.T) {
      ctx := context.Background()
      pgc, _ := postgres.Run(ctx, "postgres:16",
          postgres.WithDatabase("testdb"),
          postgres.WithUsername("app"),
          postgres.WithPassword("secret"),
          postgres.BasicWaitStrategies(),
      )
      t.Cleanup(func() { pgc.Terminate(ctx) })
      dsn, _ := pgc.ConnectionString(ctx, "sslmode=disable")
      pool, err := db.NewPool(ctx, dsn)
      if err != nil { t.Fatal(err) }
      defer pool.Close()
      if err := pool.Ping(ctx); err != nil { t.Fatal(err) }
  }
  ```
- [ ] **Step 2: Run to verify failure**
  Run: `go test ./internal/db/... -v`
  Expected: FAIL
- [ ] **Step 3: Implement**
  ```go
  // internal/db/db.go
  package db

  import (
      "context"
      "fmt"
      "github.com/jackc/pgx/v5/pgxpool"
  )

  func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
      pool, err := pgxpool.New(ctx, dsn)
      if err != nil {
          return nil, fmt.Errorf("pgxpool.New: %w", err)
      }
      if err := pool.Ping(ctx); err != nil {
          return nil, fmt.Errorf("db ping: %w", err)
      }
      return pool, nil
  }

  // CheckMigrationVersion returns an error if the DB schema is behind the
  // latest migration version. Callers should exit on error (no auto-migrate).
  func CheckMigrationVersion(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) error {
      // implemented in Task 4 alongside golang-migrate
      return nil
  }
  ```
- [ ] **Step 4: Run to verify pass**
  Run: `go test ./internal/db/... -v -timeout 120s`
  Expected: PASS
- [ ] **Step 5: Commit**
  `git add internal/db/ && git commit -m "feat(db): pgx pool setup"`

---

### Task 3: Redis

**Files:**
- Create: `internal/redis/redis.go`
- Test: `internal/redis/redis_test.go` (testcontainers-go)

- [ ] **Step 1: Write failing test**
  ```go
  // internal/redis/redis_test.go
  package redis_test

  import (
      "context"
      "testing"
      "github.com/test/taskmgr/internal/redis"
      tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
  )

  func TestNewClient(t *testing.T) {
      ctx := context.Background()
      rc, _ := tcredis.Run(ctx, "redis:7")
      t.Cleanup(func() { rc.Terminate(ctx) })
      addr, _ := rc.ConnectionString(ctx)
      client, err := redis.NewClient(addr)
      if err != nil { t.Fatal(err) }
      defer client.Close()
      if err := client.Ping(ctx).Err(); err != nil { t.Fatal(err) }
  }
  ```
- [ ] **Step 2: Run to verify failure**
  Run: `go test ./internal/redis/... -v`
  Expected: FAIL
- [ ] **Step 3: Implement**
  ```go
  // internal/redis/redis.go
  package redis

  import (
      "context"
      "fmt"
      goredis "github.com/redis/go-redis/v9"
  )

  func NewClient(addr string) (*goredis.Client, error) {
      c := goredis.NewClient(&goredis.Options{Addr: addr})
      if err := c.Ping(context.Background()).Err(); err != nil {
          return nil, fmt.Errorf("redis ping: %w", err)
      }
      return c, nil
  }
  ```
- [ ] **Step 4: Run to verify pass**
  Run: `go test ./internal/redis/... -v -timeout 60s`
  Expected: PASS
- [ ] **Step 5: Commit**
  `git add internal/redis/ && git commit -m "feat(redis): redis client setup"`

---

### Task 4: Migrations

**Files:**
- Create: `migrations/001_initial_schema.up.sql`
- Create: `migrations/001_initial_schema.down.sql`
- Modify: `internal/db/db.go` — implement `CheckMigrationVersion`
- Create: `cmd/migrate/main.go`

- [ ] **Step 1: Write failing test**
  ```go
  // internal/db/migration_test.go
  package db_test

  import (
      "context"
      "testing"
      "github.com/test/taskmgr/internal/db"
  )

  func TestCheckMigrationVersion(t *testing.T) {
      ctx := context.Background()
      // reuse container from TestNewPool — in practice share a TestMain helper
      pool := newTestPool(t, ctx)
      // before migrations, version check should error
      if err := db.CheckMigrationVersion(ctx, pool, "../../migrations"); err == nil {
          t.Fatal("expected error before migration")
      }
  }
  ```
- [ ] **Step 2: Run to verify failure**
  Run: `go test ./internal/db/... -run TestCheckMigrationVersion -v`
  Expected: FAIL
- [ ] **Step 3: Write SQL migrations**
  ```sql
  -- migrations/001_initial_schema.up.sql
  CREATE EXTENSION IF NOT EXISTS "pgcrypto";

  CREATE TABLE users (
      id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      email         TEXT NOT NULL UNIQUE,
      password_hash TEXT NOT NULL,
      display_name  TEXT NOT NULL CHECK (char_length(display_name) BETWEEN 1 AND 64),
      created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
  );

  CREATE TABLE refresh_tokens (
      id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
      token      TEXT NOT NULL UNIQUE,
      expires_at TIMESTAMPTZ NOT NULL,
      revoked_at TIMESTAMPTZ,
      created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
  );
  CREATE INDEX idx_refresh_tokens_token ON refresh_tokens(token);

  CREATE TABLE projects (
      id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      owner_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
      name        TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 128),
      description TEXT CHECK (char_length(description) <= 1024),
      deleted_at  TIMESTAMPTZ,
      created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
  );
  CREATE INDEX idx_projects_owner_id ON projects(owner_id);

  CREATE TYPE task_status AS ENUM ('todo', 'doing', 'done', 'archived');

  CREATE TABLE tasks (
      id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
      title       TEXT NOT NULL CHECK (char_length(title) BETWEEN 1 AND 256),
      status      task_status NOT NULL DEFAULT 'todo',
      priority    SMALLINT NOT NULL DEFAULT 1 CHECK (priority BETWEEN 0 AND 3),
      assignee_id UUID REFERENCES users(id) ON DELETE SET NULL,
      due_date    DATE,
      deleted_at  TIMESTAMPTZ,
      created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
      updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
  );
  CREATE INDEX idx_tasks_project_id ON tasks(project_id);
  CREATE INDEX idx_tasks_project_status ON tasks(project_id, status) WHERE deleted_at IS NULL;

  CREATE TABLE task_comments (
      id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      task_id   UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
      author_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
      body      TEXT NOT NULL CHECK (char_length(body) BETWEEN 1 AND 4096),
      created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
  );
  CREATE INDEX idx_task_comments_task_id ON task_comments(task_id, created_at DESC);
  ```
  ```sql
  -- migrations/001_initial_schema.down.sql
  DROP TABLE IF EXISTS task_comments;
  DROP TABLE IF EXISTS tasks;
  DROP TYPE IF EXISTS task_status;
  DROP TABLE IF EXISTS projects;
  DROP TABLE IF EXISTS refresh_tokens;
  DROP TABLE IF EXISTS users;
  ```
- [ ] **Step 4: Implement CheckMigrationVersion + migrate cmd**
  ```go
  // internal/db/db.go (add to existing file)
  import (
      migrate "github.com/golang-migrate/migrate/v4"
      _ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
      _ "github.com/golang-migrate/migrate/v4/source/file"
  )

  func CheckMigrationVersion(ctx context.Context, pool *pgxpool.Pool, migrationsDir string) error {
      dsn := pool.Config().ConnString()
      m, err := migrate.New("file://"+migrationsDir, "pgx5://"+dsn)
      if err != nil { return fmt.Errorf("migrate.New: %w", err) }
      defer m.Close()
      if _, _, err := m.Version(); err == migrate.ErrNilVersion {
          return fmt.Errorf("database has no migrations applied — run: taskmgr migrate up")
      } else if err != nil {
          return fmt.Errorf("migration version check: %w", err)
      }
      return nil
  }
  ```
  ```go
  // cmd/migrate/main.go
  package main

  import (
      "fmt"
      "log/slog"
      "os"
      "github.com/test/taskmgr/internal/config"
      migrate "github.com/golang-migrate/migrate/v4"
      _ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
      _ "github.com/golang-migrate/migrate/v4/source/file"
  )

  func main() {
      if len(os.Args) < 2 {
          fmt.Fprintln(os.Stderr, "usage: taskmgr migrate <up|down>")
          os.Exit(1)
      }
      cfg, err := config.Load()
      if err != nil { slog.Error("config", "err", err); os.Exit(1) }
      m, err := migrate.New("file://migrations", "pgx5://"+cfg.DBDsn)
      if err != nil { slog.Error("migrate.New", "err", err); os.Exit(1) }
      defer m.Close()
      switch os.Args[1] {
      case "up":
          if err := m.Up(); err != nil && err != migrate.ErrNoChange {
              slog.Error("migrate up", "err", err); os.Exit(1)
          }
      case "down":
          if err := m.Steps(-1); err != nil {
              slog.Error("migrate down", "err", err); os.Exit(1)
          }
      default:
          fmt.Fprintln(os.Stderr, "unknown subcommand:", os.Args[1])
          os.Exit(1)
      }
      slog.Info("migration done", "cmd", os.Args[1])
  }
  ```
- [ ] **Step 5: Run to verify pass**
  Run: `go test ./internal/db/... -v -timeout 120s`
  Expected: PASS
- [ ] **Step 6: Commit**
  `git add migrations/ internal/db/ cmd/migrate/ && git commit -m "feat(migrations): initial schema + migrate subcommand"`

---

### Task 5: Middleware

**Files:**
- Create: `internal/middleware/requestid.go`
- Create: `internal/middleware/logging.go`
- Create: `internal/middleware/auth.go`
- Test: `internal/middleware/auth_test.go`

- [ ] **Step 1: Write failing test for auth middleware**
  ```go
  // internal/middleware/auth_test.go
  package middleware_test

  import (
      "net/http"
      "net/http/httptest"
      "testing"
      "github.com/gin-gonic/gin"
      "github.com/test/taskmgr/internal/middleware"
  )

  func TestAuthMiddleware_MissingToken(t *testing.T) {
      gin.SetMode(gin.TestMode)
      r := gin.New()
      r.Use(middleware.Auth("secret"))
      r.GET("/test", func(c *gin.Context) { c.Status(200) })
      w := httptest.NewRecorder()
      req, _ := http.NewRequest("GET", "/test", nil)
      r.ServeHTTP(w, req)
      if w.Code != http.StatusUnauthorized { t.Fatalf("got %d", w.Code) }
  }
  ```
- [ ] **Step 2: Run to verify failure**
  Run: `go test ./internal/middleware/... -v`
  Expected: FAIL
- [ ] **Step 3: Implement**
  ```go
  // internal/middleware/requestid.go
  package middleware

  import (
      "github.com/gin-gonic/gin"
      "github.com/google/uuid"
  )

  const RequestIDKey = "request_id"

  func RequestID() gin.HandlerFunc {
      return func(c *gin.Context) {
          id := uuid.NewString()
          c.Set(RequestIDKey, id)
          c.Header("X-Request-ID", id)
          c.Next()
      }
  }
  ```
  ```go
  // internal/middleware/logging.go
  package middleware

  import (
      "log/slog"
      "time"
      "github.com/gin-gonic/gin"
  )

  func Logger() gin.HandlerFunc {
      return func(c *gin.Context) {
          start := time.Now()
          c.Next()
          userID, _ := c.Get("user_id")
          slog.Info("request",
              "request_id", c.GetString(RequestIDKey),
              "user_id", userID,
              "route", c.FullPath(),
              "status", c.Writer.Status(),
              "latency_ms", time.Since(start).Milliseconds(),
          )
      }
  }
  ```
  ```go
  // internal/middleware/auth.go
  package middleware

  import (
      "net/http"
      "strings"
      "github.com/gin-gonic/gin"
      "github.com/golang-jwt/jwt/v5"
  )

  func Auth(secret string) gin.HandlerFunc {
      return func(c *gin.Context) {
          header := c.GetHeader("Authorization")
          if !strings.HasPrefix(header, "Bearer ") {
              c.AbortWithStatusJSON(http.StatusUnauthorized, errorResp("unauthorized", "missing bearer token", c.GetString(RequestIDKey)))
              return
          }
          tokenStr := strings.TrimPrefix(header, "Bearer ")
          token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
              if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
                  return nil, jwt.ErrSignatureInvalid
              }
              return []byte(secret), nil
          })
          if err != nil || !token.Valid {
              c.AbortWithStatusJSON(http.StatusUnauthorized, errorResp("unauthorized", "invalid token", c.GetString(RequestIDKey)))
              return
          }
          claims, _ := token.Claims.(jwt.MapClaims)
          c.Set("user_id", claims["sub"])
          c.Next()
      }
  }

  func errorResp(code, message, reqID string) gin.H {
      return gin.H{"error": gin.H{"code": code, "message": message, "request_id": reqID}}
  }
  ```
- [ ] **Step 4: Run to verify pass**
  Run: `go test ./internal/middleware/... -v`
  Expected: PASS
- [ ] **Step 5: Commit**
  `git add internal/middleware/ && git commit -m "feat(middleware): request_id, logging, auth JWT"`

---

### Task 6: Auth Domain

**Files:**
- Create: `internal/auth/model.go`
- Create: `internal/auth/repo.go`
- Create: `internal/auth/service.go`
- Create: `internal/auth/handler.go`
- Test: `internal/auth/service_test.go` (testcontainers-go)

- [ ] **Step 1: Write failing service test**
  ```go
  // internal/auth/service_test.go
  package auth_test

  import (
      "context"
      "testing"
      "github.com/test/taskmgr/internal/auth"
      "github.com/test/taskmgr/internal/testhelper"
  )

  func TestRegisterAndLogin(t *testing.T) {
      ctx := context.Background()
      pool := testhelper.NewPool(t, ctx)
      svc := auth.NewService(auth.NewRepo(pool), "test-secret", 60, 30)
      user, err := svc.Register(ctx, "alice@example.com", "password123", "Alice")
      if err != nil { t.Fatal(err) }
      if user.Email != "alice@example.com" { t.Fatalf("got %q", user.Email) }
      pair, err := svc.Login(ctx, "alice@example.com", "password123")
      if err != nil { t.Fatal(err) }
      if pair.AccessToken == "" { t.Fatal("empty access token") }
  }
  ```
- [ ] **Step 2: Run to verify failure**
  Run: `go test ./internal/auth/... -v`
  Expected: FAIL
- [ ] **Step 3: Implement**
  Create `internal/testhelper/testhelper.go` — shared testcontainers pool + applied migrations helper.
  ```go
  // internal/testhelper/testhelper.go
  package testhelper

  import (
      "context"
      "testing"
      "github.com/jackc/pgx/v5/pgxpool"
      tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
      migrate "github.com/golang-migrate/migrate/v4"
      _ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
      _ "github.com/golang-migrate/migrate/v4/source/file"
  )

  func NewPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
      t.Helper()
      pgc, err := tcpostgres.Run(ctx, "postgres:16",
          tcpostgres.WithDatabase("testdb"),
          tcpostgres.WithUsername("app"),
          tcpostgres.WithPassword("secret"),
          tcpostgres.BasicWaitStrategies(),
      )
      if err != nil { t.Fatal(err) }
      t.Cleanup(func() { pgc.Terminate(ctx) })
      dsn, _ := pgc.ConnectionString(ctx, "sslmode=disable")
      m, _ := migrate.New("file://../../migrations", "pgx5://"+dsn)
      if err := m.Up(); err != nil && err != migrate.ErrNoChange { t.Fatal(err) }
      m.Close()
      pool, err := pgxpool.New(ctx, dsn)
      if err != nil { t.Fatal(err) }
      t.Cleanup(pool.Close)
      return pool
  }
  ```

  `internal/auth/model.go` — User and RefreshToken structs.
  `internal/auth/repo.go` — CreateUser, FindByEmail, CreateRefreshToken, FindRefreshToken, RevokeRefreshToken using pgx direct SQL.
  `internal/auth/service.go` — Register (bcrypt hash), Login (compare + issue JWT + refresh token), Refresh (validate DB token + re-issue access token), Logout (revoke token).
  `internal/auth/handler.go` — Gin handlers calling service, binding JSON, returning unified error format.
- [ ] **Step 4: Run to verify pass**
  Run: `go test ./internal/auth/... -v -timeout 120s`
  Expected: PASS
- [ ] **Step 5: Commit**
  `git add internal/auth/ internal/testhelper/ && git commit -m "feat(auth): register, login, refresh, logout"`

---

### Task 7: User Domain

**Files:**
- Create: `internal/user/repo.go`
- Create: `internal/user/handler.go`
- Test: `internal/user/handler_test.go`

- [ ] **Step 1: Write failing test**
  ```go
  // internal/user/handler_test.go
  package user_test

  // httptest GET /v1/users/me with valid JWT → 200 + correct user body
  // httptest PATCH /v1/users/me with display_name → 200 + updated body
  // httptest GET /v1/users/me without token → 401
  ```
- [ ] **Step 2: Run to verify failure**
  Run: `go test ./internal/user/... -v`
  Expected: FAIL
- [ ] **Step 3: Implement**
  `internal/user/repo.go` — FindByID (pgx), UpdateDisplayName.
  `internal/user/handler.go` — GET/PATCH /v1/users/me, reads user_id from gin.Context.
- [ ] **Step 4: Run to verify pass**
  Run: `go test ./internal/user/... -v -timeout 120s`
  Expected: PASS
- [ ] **Step 5: Commit**
  `git add internal/user/ && git commit -m "feat(user): GET/PATCH /v1/users/me"`

---

### Task 8: Project Domain

**Files:**
- Create: `internal/project/model.go`
- Create: `internal/project/repo.go`
- Create: `internal/project/handler.go`
- Test: `internal/project/handler_test.go`

- [ ] **Step 1: Write failing test**
  ```go
  // Test: create project → list → get → patch → delete → list returns empty
  // Test: user B cannot GET user A's project (404)
  // Test: cursor pagination returns correct next_cursor
  ```
- [ ] **Step 2: Run to verify failure**
  Run: `go test ./internal/project/... -v`
  Expected: FAIL
- [ ] **Step 3: Implement**
  `internal/project/repo.go` — Create, List (cursor: `WHERE owner_id=$1 AND (created_at, id) < ($cursor_ts, $cursor_id) AND deleted_at IS NULL ORDER BY created_at DESC, id DESC LIMIT $limit+1`), FindByIDAndOwner, Update, SoftDelete (sets deleted_at, calls task repo soft-delete-by-project in same transaction).
  `internal/project/handler.go` — five handlers; cursor encoded as `base64(created_at.Format(time.RFC3339Nano) + "," + id)`.
- [ ] **Step 4: Run to verify pass**
  Run: `go test ./internal/project/... -v -timeout 120s`
  Expected: PASS
- [ ] **Step 5: Commit**
  `git add internal/project/ && git commit -m "feat(project): CRUD with cursor pagination and soft delete"`

---

### Task 9: Task Domain

**Files:**
- Create: `internal/task/model.go`
- Create: `internal/task/statemachine.go`
- Create: `internal/task/repo.go`
- Create: `internal/task/handler.go`
- Test: `internal/task/statemachine_test.go`
- Test: `internal/task/handler_test.go`

- [ ] **Step 1: Write failing statemachine test**
  ```go
  // internal/task/statemachine_test.go
  package task_test

  import (
      "testing"
      "github.com/test/taskmgr/internal/task"
  )

  func TestTransitions(t *testing.T) {
      cases := []struct{ from, to task.Status; ok bool }{
          {"todo", "doing", true},
          {"doing", "done", true},
          {"done", "archived", true},
          {"done", "doing", true},   // reopen allowed
          {"todo", "archived", false},
          {"archived", "todo", false},
          {"doing", "todo", false},
      }
      for _, c := range cases {
          err := task.ValidateTransition(c.from, c.to)
          if c.ok && err != nil { t.Errorf("%s→%s: expected ok, got %v", c.from, c.to, err) }
          if !c.ok && err == nil { t.Errorf("%s→%s: expected error", c.from, c.to) }
      }
  }
  ```
- [ ] **Step 2: Run to verify failure**
  Run: `go test ./internal/task/... -run TestTransitions -v`
  Expected: FAIL
- [ ] **Step 3: Implement statemachine**
  ```go
  // internal/task/statemachine.go
  package task

  import "fmt"

  type Status string
  const (
      StatusTodo     Status = "todo"
      StatusDoing    Status = "doing"
      StatusDone     Status = "done"
      StatusArchived Status = "archived"
  )

  var allowed = map[Status]map[Status]bool{
      StatusTodo:     {StatusDoing: true},
      StatusDoing:    {StatusDone: true},
      StatusDone:     {StatusArchived: true, StatusDoing: true},
      StatusArchived: {},
  }

  func ValidateTransition(from, to Status) error {
      if allowed[from][to] { return nil }
      return fmt.Errorf("task_invalid_transition: %s → %s", from, to)
  }
  ```
- [ ] **Step 4: Implement repo + handler**
  `internal/task/repo.go` — Create, ListByProject (cursor paged, filter by status/assignee_id, WHERE deleted_at IS NULL), FindByIDAndProject (join project to verify ownership), Update, SoftDelete, SoftDeleteByProject (for cascade).
  `internal/task/handler.go` — 6 handlers; TransitionTask calls ValidateTransition then repo.Update; assignee change triggers notify event.
- [ ] **Step 5: Run to verify pass**
  Run: `go test ./internal/task/... -v -timeout 120s`
  Expected: PASS
- [ ] **Step 6: Commit**
  `git add internal/task/ && git commit -m "feat(task): CRUD + state machine transitions"`

---

### Task 10: Comment Domain

**Files:**
- Create: `internal/comment/repo.go`
- Create: `internal/comment/handler.go`
- Test: `internal/comment/handler_test.go`

- [ ] **Step 1: Write failing test**
  ```go
  // Test: post comment → 201
  // Test: 4th comment within 10s → 429 rate_limited
  // Test: list comments returns cursor-paginated results
  ```
- [ ] **Step 2: Run to verify failure**
  Run: `go test ./internal/comment/... -v`
  Expected: FAIL
- [ ] **Step 3: Implement**
  Rate limiting: before inserting, call `INCR ratelimit:comment:{userID}:{taskID}` then `EXPIRE … 10`; if result > 3 return 429.
  `internal/comment/repo.go` — Create, List (cursor by created_at DESC, id DESC).
  `internal/comment/handler.go` — POST checks ownership via task→project→owner join, then rate-limits, then inserts.
- [ ] **Step 4: Run to verify pass**
  Run: `go test ./internal/comment/... -v -timeout 120s`
  Expected: PASS
- [ ] **Step 5: Commit**
  `git add internal/comment/ && git commit -m "feat(comment): post/list with Redis rate limiting"`

---

### Task 11: Stats Domain

**Files:**
- Create: `internal/stats/repo.go`
- Create: `internal/stats/handler.go`
- Test: `internal/stats/handler_test.go`

- [ ] **Step 1: Write failing test**
  ```go
  // Test: create tasks in various statuses + overdue, call stats → correct counts
  // Test: second call returns cached result (verify single DB query via mock or counter)
  // Test: after project delete, stats returns 404
  ```
- [ ] **Step 2: Run to verify failure**
  Run: `go test ./internal/stats/... -v`
  Expected: FAIL
- [ ] **Step 3: Implement**
  ```go
  // internal/stats/repo.go
  // SELECT
  //   COUNT(*) FILTER (WHERE status='todo') AS todo,
  //   COUNT(*) FILTER (WHERE status='doing') AS doing,
  //   COUNT(*) FILTER (WHERE status='done') AS done,
  //   COUNT(*) FILTER (WHERE status='archived') AS archived,
  //   COUNT(*) FILTER (WHERE due_date < CURRENT_DATE AND status IN ('todo','doing')) AS overdue
  // FROM tasks
  // WHERE project_id=$1 AND deleted_at IS NULL
  ```
  Cache key: `stats:{projectID}`. On GET: try Redis GET → on miss query DB → SET EX 60. Project soft-delete must DEL this key (call from project repo within the same transaction's cleanup).
- [ ] **Step 4: Run to verify pass**
  Run: `go test ./internal/stats/... -v -timeout 120s`
  Expected: PASS
- [ ] **Step 5: Commit**
  `git add internal/stats/ && git commit -m "feat(stats): aggregation with 60s Redis cache"`

---

### Task 12: Health Domain

**Files:**
- Create: `internal/health/handler.go`
- Test: `internal/health/handler_test.go`

- [ ] **Step 1: Write failing test**
  ```go
  // Test: /healthz always 200
  // Test: /readyz 200 when DB + Redis up
  // Test: /readyz 503 when DB pool.Ping fails
  ```
- [ ] **Step 2: Run to verify failure**
  Run: `go test ./internal/health/... -v`
  Expected: FAIL
- [ ] **Step 3: Implement**
  ```go
  // internal/health/handler.go
  package health

  import (
      "context"
      "net/http"
      "time"
      "github.com/gin-gonic/gin"
      "github.com/jackc/pgx/v5/pgxpool"
      goredis "github.com/redis/go-redis/v9"
  )

  type Handler struct { pool *pgxpool.Pool; redis *goredis.Client }

  func NewHandler(pool *pgxpool.Pool, redis *goredis.Client) *Handler {
      return &Handler{pool: pool, redis: redis}
  }

  func (h *Handler) Healthz(c *gin.Context) {
      c.JSON(http.StatusOK, gin.H{"status": "ok"})
  }

  func (h *Handler) Readyz(c *gin.Context) {
      ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
      defer cancel()
      if err := h.pool.Ping(ctx); err != nil {
          c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "db_unreachable", "message": err.Error()}})
          return
      }
      if err := h.redis.Ping(ctx).Err(); err != nil {
          c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "redis_unreachable", "message": err.Error()}})
          return
      }
      c.JSON(http.StatusOK, gin.H{"status": "ok"})
  }
  ```
- [ ] **Step 4: Run to verify pass**
  Run: `go test ./internal/health/... -v -timeout 60s`
  Expected: PASS
- [ ] **Step 5: Commit**
  `git add internal/health/ && git commit -m "feat(health): /healthz and /readyz probes"`

---

### Task 13: Notify

**Files:**
- Create: `internal/notify/notify.go`
- Test: `internal/notify/notify_test.go`

- [ ] **Step 1: Write failing test**
  ```go
  // Test: publish AssignedEvent → consumer logs it within 100ms
  ```
- [ ] **Step 2: Run to verify failure**
  Run: `go test ./internal/notify/... -v`
  Expected: FAIL
- [ ] **Step 3: Implement**
  ```go
  // internal/notify/notify.go
  package notify

  import "log/slog"

  type AssignedEvent struct {
      TaskID     string
      AssigneeID string
  }

  type Notifier struct { ch chan AssignedEvent }

  func New(bufSize int) *Notifier { return &Notifier{ch: make(chan AssignedEvent, bufSize)} }

  // Publish enqueues an event non-blocking; drops if buffer full (fire-and-forget).
  func (n *Notifier) Publish(e AssignedEvent) {
      select {
      case n.ch <- e:
      default:
          slog.Warn("notify: channel full, dropping event", "task_id", e.TaskID)
      }
  }

  // Run consumes events until ctx is cancelled. Call as goroutine from buildApp.
  func (n *Notifier) Run(ctx interface{ Done() <-chan struct{} }) {
      for {
          select {
          case e := <-n.ch:
              slog.Info("assigned", "task_id", e.TaskID, "assignee_id", e.AssigneeID)
          case <-ctx.(interface{ Done() <-chan struct{} }).Done():
              return
          }
      }
  }
  ```
- [ ] **Step 4: Run to verify pass**
  Run: `go test ./internal/notify/... -v`
  Expected: PASS
- [ ] **Step 5: Commit**
  `git add internal/notify/ && git commit -m "feat(notify): async assignee-change event channel"`

---

### Task 14: Wire — buildApp + graceful shutdown

**Files:**
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Write failing integration smoke test**
  ```go
  // cmd/server/main_test.go
  // Start server on :0, send GET /healthz, expect 200
  ```
- [ ] **Step 2: Run to verify failure**
  Run: `go test ./cmd/server/... -v`
  Expected: FAIL (buildApp only stubs healthz)
- [ ] **Step 3: Implement full buildApp**
  Wire all domain handlers into the router. Mount routes:
  - `POST /v1/auth/register`, `POST /v1/auth/login`, `POST /v1/auth/refresh`, `POST /v1/auth/logout`
  - Auth middleware group for all `/v1/users/*`, `/v1/projects/*`, `/v1/tasks/*`
  - `GET /healthz`, `GET /readyz`
  
  Graceful shutdown: change `context.WithTimeout` to `30*time.Second`. Add cleanup to close pgxpool + redis + notifier goroutine.

  Startup: call `db.CheckMigrationVersion`; if err, `slog.Error + os.Exit(1)`.
- [ ] **Step 4: Run to verify pass**
  Run: `go test ./cmd/server/... -v -timeout 120s`
  Expected: PASS
- [ ] **Step 5: Commit**
  `git add cmd/server/ && git commit -m "feat(wire): full buildApp with graceful shutdown"`

---

### Task 15: oapi-codegen stubs

**Files:**
- Create: `api/gen/server.gen.go`, `api/gen/types.gen.go` (generated)
- Create: `api/generate.go` (go:generate directive)

- [ ] **Step 1: Create go:generate file**
  ```go
  // api/generate.go
  package api

  //go:generate oapi-codegen -generate gin,types -package api -o gen/server.gen.go ../openapi.yaml
  ```
- [ ] **Step 2: Run generation**
  Run: `go generate ./api/...`
  Expected: `api/gen/server.gen.go` created with `StrictServerInterface`
- [ ] **Step 3: Verify handlers satisfy interface**
  Add compile-time assertion in `cmd/server/main.go`:
  ```go
  // var _ api.StrictServerInterface = (*server)(nil)
  ```
  Ensure all handler structs satisfy the generated interface.
- [ ] **Step 4: Commit**
  `git add api/ && git commit -m "feat(api): oapi-codegen server stubs"`

---

### Task 16: Integration Tests

**Files:**
- Create: `internal/integration/suite_test.go`
- Create: `internal/integration/scenarios_test.go`

- [ ] **Step 1: Write all 10 acceptance scenario tests**
  Each test spins up testcontainers Postgres + Redis, runs `migrate up`, starts the HTTP server on `:0` via `httptest.NewServer`, then exercises the full API flow.
  
  ```go
  // Scenario 1: happy path register→login→project→task→done→stats
  func TestHappyPath(t *testing.T) { ... }

  // Scenario 2: cross-user 404
  func TestCrossUserIsolation(t *testing.T) { ... }

  // Scenario 3: comment rate limit 429
  func TestCommentRateLimit(t *testing.T) { ... }

  // Scenario 4: invalid transition todo→archived = 409
  func TestInvalidTransition(t *testing.T) { ... }

  // Scenario 5: delete project cascades + cache cleared
  func TestProjectDeleteCascade(t *testing.T) { ... }

  // Scenario 6: overdue counted in stats
  func TestOverdueStats(t *testing.T) { ... }

  // Scenario 7: graceful shutdown
  func TestGracefulShutdown(t *testing.T) { ... }

  // Scenario 8: readyz 503 when DB down
  func TestReadyzDBDown(t *testing.T) { ... }

  // Scenario 9: expired refresh token → 401
  func TestExpiredRefreshToken(t *testing.T) { ... }

  // Scenario 10: assignee change emits notify log
  func TestAssigneeNotifyLog(t *testing.T) { ... }
  ```
- [ ] **Step 2: Run all integration tests**
  Run: `go test ./internal/integration/... -v -timeout 300s`
  Expected: all PASS
- [ ] **Step 3: Commit**
  `git add internal/integration/ && git commit -m "test(integration): all 10 acceptance scenarios"`

---

### Task 17: Migrate Subcommand (finalize)

Already implemented in Task 4. This task verifies the binary builds and runs end-to-end.

- [ ] **Step 1: Build binary**
  Run: `go build -o dist/taskmgr-migrate ./cmd/migrate/`
  Expected: binary produced, no errors
- [ ] **Step 2: Commit**
  `git add dist/ Makefile && git commit -m "build: migrate subcommand binary"`

---

### Task 18: Docker Compose (service entry)

**Files:**
- Modify: `docker-compose.yml`

- [ ] **Step 1: Add service entry**
  ```yaml
  # docker-compose.yml (add to services:)
  app:
    build: .
    ports:
      - "8080:8080"
    environment:
      TASKMGR_DB_DSN: postgres://app:secret@postgres:5432/appdb?sslmode=disable
      TASKMGR_REDIS_ADDR: redis:6379
      TASKMGR_JWT_SECRET: changeme-in-prod
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
  ```
  Also create `Dockerfile`:
  ```dockerfile
  FROM golang:1.25-alpine AS build
  WORKDIR /app
  COPY . .
  RUN go build -o /taskmgr ./cmd/server/

  FROM alpine:3.21
  COPY --from=build /taskmgr /taskmgr
  COPY migrations/ /migrations/
  ENTRYPOINT ["/taskmgr"]
  ```
- [ ] **Step 2: Commit**
  `git add docker-compose.yml Dockerfile && git commit -m "build: Dockerfile and compose service entry"`

---

### Task 19: README

**Files:**
- Modify: `docs/overview.md` or create `README.md`

- [ ] **Step 1: Write README**
  Sections: Prerequisites, Quick start (`docker-compose up`), Run migrations (`go run ./cmd/migrate up`), Run tests (`go test ./... -timeout 300s`), Generate client (`oapi-codegen ...`), Environment variables table.
- [ ] **Step 2: Commit**
  `git add README.md && git commit -m "docs: README with local start, test, migrate, client-gen"`

---

## Schema changes

- `users`: new table — `id uuid PK`, `email text UNIQUE NOT NULL`, `password_hash text NOT NULL`, `display_name text NOT NULL`, `created_at timestamptz NOT NULL DEFAULT NOW()`
- `refresh_tokens`: new table — `id uuid PK`, `user_id uuid FK→users`, `token text UNIQUE NOT NULL`, `expires_at timestamptz NOT NULL`, `revoked_at timestamptz`, `created_at timestamptz NOT NULL DEFAULT NOW()`; index on `token`
- `projects`: new table — `id uuid PK`, `owner_id uuid FK→users`, `name text NOT NULL`, `description text`, `deleted_at timestamptz`, `created_at timestamptz NOT NULL DEFAULT NOW()`; index on `owner_id`
- `tasks`: new table — `id uuid PK`, `project_id uuid FK→projects`, `title text NOT NULL`, `status task_status NOT NULL DEFAULT 'todo'`, `priority smallint NOT NULL DEFAULT 1`, `assignee_id uuid FK→users nullable`, `due_date date`, `deleted_at timestamptz`, `created_at/updated_at timestamptz`; composite index `(project_id, status) WHERE deleted_at IS NULL`
- `task_comments`: new table — `id uuid PK`, `task_id uuid FK→tasks`, `author_id uuid FK→users`, `body text NOT NULL`, `created_at timestamptz NOT NULL DEFAULT NOW()`; index on `(task_id, created_at DESC)`
- `task_status` enum: new type — values `todo`, `doing`, `done`, `archived`
