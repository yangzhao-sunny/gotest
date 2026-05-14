//go:build integration

package db

import (
	"context"
	"testing"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestCheckMigrationVersion_Integration(t *testing.T) {
	ctx := context.Background()
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

	// before migrations: expect error
	if err := CheckMigrationVersion(ctx, dsn, "../../migrations"); err == nil {
		t.Fatal("expected error before any migrations applied")
	}

	// apply migrations
	pool, err := NewPool(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// apply via migrate CLI equivalent
	applyMigrations(t, dsn)

	// after migrations: expect no error
	if err := CheckMigrationVersion(ctx, dsn, "../../migrations"); err != nil {
		t.Fatalf("expected no error after migrations: %v", err)
	}
}

func applyMigrations(t *testing.T, dsn string) {
	t.Helper()
	m, err := newMigrateInstance("../../migrations", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.Up(); err != nil {
		t.Fatal(err)
	}
}
