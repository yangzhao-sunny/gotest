package health

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
)

type Handler struct {
	pool  *pgxpool.Pool
	redis *goredis.Client
}

func NewHandler(pool *pgxpool.Pool, redis *goredis.Client) *Handler {
	return &Handler{pool: pool, redis: redis}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/healthz", h.Healthz)
	rg.GET("/readyz", h.Readyz)
}

func (h *Handler) Healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) Readyz(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	if err := h.pool.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "detail": "db unavailable"})
		return
	}

	if err := h.redis.Ping(ctx).Err(); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "error", "detail": "redis unavailable"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
