package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	router, cleanup, err := buildApp()
	if err != nil {
		slog.Error("failed to initialize app", "err", err)
		os.Exit(1)
	}
	defer cleanup()

	srv := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "err", err)
	}
}

// buildApp wires the application graph by hand.
// Add new dependencies here as features are introduced:
//
//	cfg := config.Load(...)
//	pool := db.NewPool(cfg.DB)
//	userRepo := user.NewRepo(pool)
//	userSvc  := user.NewService(userRepo)
//	userH    := user.NewHandler(userSvc)
//	r.POST("/users", userH.Create)
//
// Keep construction explicit; no service-locator / global state.
func buildApp() (*gin.Engine, func(), error) {
	r := gin.Default()
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	cleanup := func() {}
	return r, cleanup, nil
}
