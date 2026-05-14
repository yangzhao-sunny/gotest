package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db ping: %w", err)
	}
	return pool, nil
}

// toPgx5DSN converts any postgres:// or postgresql:// URL to pgx5:// for golang-migrate.
func toPgx5DSN(dsn string) string {
	for _, prefix := range []string{"postgresql://", "postgres://"} {
		if strings.HasPrefix(dsn, prefix) {
			return "pgx5://" + dsn[len(prefix):]
		}
	}
	return "pgx5://" + dsn
}

func newMigrateInstance(migrationsDir, dsn string) (*migrate.Migrate, error) {
	return migrate.New("file://"+migrationsDir, toPgx5DSN(dsn))
}

// CheckMigrationVersion returns an error if no migrations have been applied,
// signalling that the operator must run `taskmgr migrate up` before starting.
func CheckMigrationVersion(_ context.Context, dsn string, migrationsDir string) error {
	m, err := newMigrateInstance(migrationsDir, dsn)
	if err != nil {
		return fmt.Errorf("migrate.New: %w", err)
	}
	defer m.Close()
	_, _, err = m.Version()
	if err == migrate.ErrNilVersion {
		return fmt.Errorf("database has no migrations applied — run: taskmgr migrate up")
	}
	if err != nil {
		return fmt.Errorf("migration version check: %w", err)
	}
	return nil
}
