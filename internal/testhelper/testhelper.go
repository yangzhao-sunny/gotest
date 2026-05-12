package testhelper

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

func toPgx5DSN(dsn string) string {
	for _, prefix := range []string{"postgresql://", "postgres://"} {
		if strings.HasPrefix(dsn, prefix) {
			return "pgx5://" + dsn[len(prefix):]
		}
	}
	return "pgx5://" + dsn
}

// migrationsDir returns the absolute path to the project's migrations/ directory,
// regardless of which package's test is calling this helper.
func migrationsDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "migrations")
}

// NewPoolWithDSN returns both the pool and the raw DSN (postgres:// URL).
func NewPoolWithDSN(t *testing.T, ctx context.Context) (*pgxpool.Pool, string) {
	t.Helper()
	pgc, err := tcpostgres.Run(ctx, "postgres:16",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("app"),
		tcpostgres.WithPassword("secret"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pgc.Terminate(ctx) })

	dsn, err := pgc.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}

	m, err := migrate.New("file://"+migrationsDir(), toPgx5DSN(dsn))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatal(err)
	}
	m.Close()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, dsn
}

func NewPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	pgc, err := tcpostgres.Run(ctx, "postgres:16",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("app"),
		tcpostgres.WithPassword("secret"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pgc.Terminate(ctx) })

	dsn, err := pgc.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}

	migDSN := toPgx5DSN(dsn)
	m, err := migrate.New("file://"+migrationsDir(), migDSN)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatal(err)
	}
	m.Close()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func NewRedis(t *testing.T, ctx context.Context) *goredis.Client {
	t.Helper()
	rc, err := tcredis.Run(ctx, "redis:7")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rc.Terminate(ctx) })

	addr, err := rc.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// strip redis:// scheme
	for _, prefix := range []string{"redis://", "rediss://"} {
		if len(addr) > len(prefix) && addr[:len(prefix)] == prefix {
			addr = addr[len(prefix):]
			break
		}
	}

	client := goredis.NewClient(&goredis.Options{Addr: addr})
	t.Cleanup(func() { client.Close() })
	return client
}
