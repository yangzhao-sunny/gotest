package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/test/taskmgr/internal/config"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: taskmgr-migrate <up|down>")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}

	m, err := migrate.New("file://migrations", "pgx5://"+cfg.DBDsn)
	if err != nil {
		slog.Error("migrate.New", "err", err)
		os.Exit(1)
	}
	defer m.Close()

	switch os.Args[1] {
	case "up":
		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			slog.Error("migrate up", "err", err)
			os.Exit(1)
		}
	case "down":
		if err := m.Steps(-1); err != nil {
			slog.Error("migrate down", "err", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown subcommand:", os.Args[1])
		os.Exit(1)
	}

	slog.Info("migration done", "cmd", os.Args[1])
}
