package stats

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/test/taskmgr/internal/middleware"
)

type Handler struct {
	repo  *Repo
	pool  *pgxpool.Pool
	redis *goredis.Client
}

func NewHandler(repo *Repo, pool *pgxpool.Pool, redis *goredis.Client) *Handler {
	return &Handler{repo: repo, pool: pool, redis: redis}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/projects/:id/stats", h.GetStats)
}

func (h *Handler) GetStats(c *gin.Context) {
	userID := c.GetString(middleware.UserIDKey)
	projectID := c.Param("id")

	// Verify project ownership
	var exists bool
	err := h.pool.QueryRow(c.Request.Context(),
		`SELECT EXISTS(
		   SELECT 1 FROM projects
		   WHERE id = $1 AND owner_id = $2 AND deleted_at IS NULL
		 )`,
		projectID, userID,
	).Scan(&exists)
	if err != nil && err != pgx.ErrNoRows {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, apiErr("not_found", "project not found", reqID(c)))
		return
	}

	cacheKey := "stats:" + projectID

	// Try Redis cache
	cached, err := h.redis.Get(c.Request.Context(), cacheKey).Bytes()
	if err == nil {
		var s Stats
		if jsonErr := json.Unmarshal(cached, &s); jsonErr == nil {
			c.JSON(http.StatusOK, &s)
			return
		}
	}

	// Query DB
	s, err := h.repo.GetStats(c.Request.Context(), projectID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}

	// Cache in Redis
	if data, err := json.Marshal(s); err == nil {
		h.redis.Set(c.Request.Context(), cacheKey, data, 60*time.Second)
	}

	c.JSON(http.StatusOK, s)
}

// InvalidateCache removes the stats cache entry for a project.
func (h *Handler) InvalidateCache(ctx context.Context, projectID string) error {
	return h.redis.Del(ctx, fmt.Sprintf("stats:%s", projectID)).Err()
}

func reqID(c *gin.Context) string { return c.GetString(middleware.RequestIDKey) }

func apiErr(code, message, requestID string) gin.H {
	return gin.H{"error": gin.H{"code": code, "message": message, "request_id": requestID}}
}
