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
	"github.com/test/taskmgr/internal/auth"
	"github.com/test/taskmgr/internal/comment"
	"github.com/test/taskmgr/internal/config"
	"github.com/test/taskmgr/internal/db"
	"github.com/test/taskmgr/internal/health"
	"github.com/test/taskmgr/internal/middleware"
	"github.com/test/taskmgr/internal/notify"
	"github.com/test/taskmgr/internal/project"
	iRedis "github.com/test/taskmgr/internal/redis"
	"github.com/test/taskmgr/internal/stats"
	"github.com/test/taskmgr/internal/task"
	"github.com/test/taskmgr/internal/user"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	port := config.EnvOr("TASKMGR_PORT", "8080")

	router, cleanup, err := buildApp()
	if err != nil {
		slog.Error("failed to initialize app", "err", err)
		os.Exit(1)
	}
	defer cleanup()

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: router,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
		}
	}()
	slog.Info("server started", "port", port)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "err", err)
	}
}

func buildApp() (*gin.Engine, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}

	ctx := context.Background()

	pool, err := db.NewPool(ctx, cfg.DBDsn)
	if err != nil {
		return nil, nil, err
	}

	if err := db.CheckMigrationVersion(ctx, cfg.DBDsn, "migrations"); err != nil {
		pool.Close()
		return nil, nil, err
	}

	redisClient, err := iRedis.NewClient(cfg.RedisAddr)
	if err != nil {
		pool.Close()
		return nil, nil, err
	}

	notifier := notify.New(256)
	notifyCtx, notifyCancel := context.WithCancel(context.Background())
	go notifier.Run(notifyCtx)

	cleanup := func() {
		notifyCancel()
		redisClient.Close()
		pool.Close()
	}

	// Repos
	authRepo := auth.NewRepo(pool)
	userRepo := user.NewRepo(pool)
	projRepo := project.NewRepo(pool)
	taskRepo := task.NewRepo(pool)
	commentRepo := comment.NewRepo(pool)
	statsRepo := stats.NewRepo(pool)

	// Services
	authSvc := auth.NewService(authRepo, cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)

	// Handlers
	authH := auth.NewHandler(authSvc)
	userH := user.NewHandler(userRepo)
	projH := project.NewHandler(projRepo, taskRepo, redisClient)
	taskH := task.NewHandler(taskRepo, projRepo, notifier)
	commentH := comment.NewHandler(commentRepo, redisClient)
	statsH := stats.NewHandler(statsRepo, pool, redisClient)
	healthH := health.NewHandler(pool, redisClient)

	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Logger())

	// Public
	r.GET("/healthz", healthH.Healthz)
	r.GET("/readyz", healthH.Readyz)

	v1 := r.Group("/v1")
	v1.POST("/auth/register", authH.Register)
	v1.POST("/auth/login", authH.Login)
	v1.POST("/auth/refresh", authH.Refresh)

	// Protected
	protected := v1.Group("", middleware.Auth(cfg.JWTSecret))
	protected.POST("/auth/logout", authH.Logout)

	userH.RegisterRoutes(protected)
	projH.RegisterRoutes(protected)
	taskH.RegisterRoutes(protected)
	commentH.RegisterRoutes(protected)
	statsH.RegisterRoutes(protected)

	return r, cleanup, nil
}
