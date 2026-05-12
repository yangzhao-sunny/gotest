package comment

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"github.com/test/taskmgr/internal/middleware"
)

type Handler struct {
	repo  *Repo
	redis *goredis.Client
}

func NewHandler(repo *Repo, redis *goredis.Client) *Handler {
	return &Handler{repo: repo, redis: redis}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/tasks/:id/comments", h.Create)
	rg.GET("/tasks/:id/comments", h.List)
}

func (h *Handler) Create(c *gin.Context) {
	userID := c.GetString(middleware.UserIDKey)
	taskID := c.Param("id")

	// Check task ownership
	exists, err := h.repo.TaskExistsForOwner(c.Request.Context(), taskID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, apiErr("not_found", "task not found", reqID(c)))
		return
	}

	// Rate limiting — pipeline makes INCR+EXPIRE atomic under failure
	key := fmt.Sprintf("ratelimit:comment:%s:%s", userID, taskID)
	pipe := h.redis.Pipeline()
	incrCmd := pipe.Incr(c.Request.Context(), key)
	pipe.Expire(c.Request.Context(), key, 10*time.Second)
	if _, err = pipe.Exec(c.Request.Context()); err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", "rate limit check failed", reqID(c)))
		return
	}
	count := incrCmd.Val()
	if count > 3 {
		c.JSON(http.StatusTooManyRequests, apiErr("rate_limited", "too many comments in this window", reqID(c)))
		return
	}

	var req struct {
		Body string `json:"body" binding:"required,min=1,max=4096"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, apiErr("validation_error", err.Error(), reqID(c)))
		return
	}

	comment, err := h.repo.Create(c.Request.Context(), taskID, userID, req.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}
	c.JSON(http.StatusCreated, commentResp(comment))
}

func (h *Handler) List(c *gin.Context) {
	userID := c.GetString(middleware.UserIDKey)
	taskID := c.Param("id")

	// Check task ownership
	exists, err := h.repo.TaskExistsForOwner(c.Request.Context(), taskID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}
	if !exists {
		c.JSON(http.StatusNotFound, apiErr("not_found", "task not found", reqID(c)))
		return
	}

	cursor := c.Query("cursor")
	limit := 20
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	comments, nextCursor, err := h.repo.List(c.Request.Context(), taskID, cursor, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}

	var data []gin.H
	for _, cm := range comments {
		data = append(data, commentResp(cm))
	}
	if data == nil {
		data = []gin.H{}
	}
	resp := gin.H{"data": data, "next_cursor": nil}
	if nextCursor != "" {
		resp["next_cursor"] = nextCursor
	}
	c.JSON(http.StatusOK, resp)
}

func commentResp(c *Comment) gin.H {
	return gin.H{
		"id":         c.ID,
		"task_id":    c.TaskID,
		"author_id":  c.AuthorID,
		"body":       c.Body,
		"created_at": c.CreatedAt,
	}
}

func reqID(c *gin.Context) string { return c.GetString(middleware.RequestIDKey) }

func apiErr(code, message, requestID string) gin.H {
	return gin.H{"error": gin.H{"code": code, "message": message, "request_id": requestID}}
}
