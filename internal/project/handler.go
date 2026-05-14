package project

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"github.com/test/taskmgr/internal/middleware"
)

type Handler struct {
	repo        *Repo
	taskDeleter TaskDeleter
	redis       *goredis.Client
}

func NewHandler(repo *Repo, taskDeleter TaskDeleter, redis *goredis.Client) *Handler {
	return &Handler{repo: repo, taskDeleter: taskDeleter, redis: redis}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/projects", h.Create)
	rg.GET("/projects", h.List)
	rg.GET("/projects/:id", h.Get)
	rg.PATCH("/projects/:id", h.Update)
	rg.DELETE("/projects/:id", h.Delete)
}

func (h *Handler) Create(c *gin.Context) {
	var req struct {
		Name        string  `json:"name" binding:"required,min=1,max=128"`
		Description *string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, apiErr("validation_error", err.Error(), reqID(c)))
		return
	}
	userID := c.GetString(middleware.UserIDKey)
	p, err := h.repo.Create(c.Request.Context(), userID, req.Name, req.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}
	c.JSON(http.StatusCreated, projectResp(p))
}

func (h *Handler) List(c *gin.Context) {
	userID := c.GetString(middleware.UserIDKey)
	cursor := c.Query("cursor")
	limit := 20
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	projects, nextCursor, err := h.repo.List(c.Request.Context(), userID, cursor, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}
	var data []gin.H
	for _, p := range projects {
		data = append(data, projectResp(p))
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

func (h *Handler) Get(c *gin.Context) {
	userID := c.GetString(middleware.UserIDKey)
	id := c.Param("id")
	p, err := h.repo.FindByIDAndOwner(c.Request.Context(), id, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, apiErr("not_found", "project not found", reqID(c)))
		return
	}
	c.JSON(http.StatusOK, projectResp(p))
}

func (h *Handler) Update(c *gin.Context) {
	var req struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, apiErr("validation_error", err.Error(), reqID(c)))
		return
	}
	userID := c.GetString(middleware.UserIDKey)
	id := c.Param("id")
	p, err := h.repo.Update(c.Request.Context(), id, userID, req.Name, req.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, apiErr("not_found", "project not found", reqID(c)))
		return
	}
	c.JSON(http.StatusOK, projectResp(p))
}

func (h *Handler) Delete(c *gin.Context) {
	userID := c.GetString(middleware.UserIDKey)
	id := c.Param("id")

	// Check exists first for 404
	p, err := h.repo.FindByIDAndOwner(c.Request.Context(), id, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, apiErr("not_found", "project not found", reqID(c)))
		return
	}

	if err := h.repo.SoftDelete(c.Request.Context(), id, userID, h.taskDeleter); err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}

	// Invalidate stats cache
	if h.redis != nil {
		h.redis.Del(context.Background(), "stats:"+id)
	}

	c.Status(http.StatusNoContent)
}

func projectResp(p *Project) gin.H {
	return gin.H{
		"id":          p.ID,
		"owner_id":    p.OwnerID,
		"name":        p.Name,
		"description": p.Description,
		"created_at":  p.CreatedAt,
	}
}

func reqID(c *gin.Context) string { return c.GetString(middleware.RequestIDKey) }

func apiErr(code, message, requestID string) gin.H {
	return gin.H{"error": gin.H{"code": code, "message": message, "request_id": requestID}}
}
