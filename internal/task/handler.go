package task

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/test/taskmgr/internal/middleware"
	"github.com/test/taskmgr/internal/project"
)

// Notifier is called when a task's assignee changes.
type Notifier interface {
	Publish(taskID, assigneeID string)
}

type Handler struct {
	repo      *Repo
	projRepo  *project.Repo
	notifier  Notifier
}

func NewHandler(repo *Repo, projRepo *project.Repo, notifier Notifier) *Handler {
	return &Handler{repo: repo, projRepo: projRepo, notifier: notifier}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/projects/:id/tasks", h.Create)
	rg.GET("/projects/:id/tasks", h.List)
	rg.GET("/tasks/:id", h.Get)
	rg.PATCH("/tasks/:id", h.Update)
	rg.DELETE("/tasks/:id", h.Delete)
	rg.POST("/tasks/:id/transitions", h.Transition)
}

func (h *Handler) Create(c *gin.Context) {
	userID := c.GetString(middleware.UserIDKey)
	projectID := c.Param("id")

	// Verify project ownership
	p, err := h.projRepo.FindByIDAndOwner(c.Request.Context(), projectID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, apiErr("not_found", "project not found", reqID(c)))
		return
	}

	var req struct {
		Title      string  `json:"title" binding:"required,min=1,max=256"`
		Priority   int     `json:"priority"`
		AssigneeID *string `json:"assignee_id"`
		DueDate    *string `json:"due_date"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, apiErr("validation_error", err.Error(), reqID(c)))
		return
	}

	t, err := h.repo.Create(c.Request.Context(), projectID, req.Title, req.Priority, req.AssigneeID, req.DueDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}
	c.JSON(http.StatusCreated, taskResp(t))
}

func (h *Handler) List(c *gin.Context) {
	userID := c.GetString(middleware.UserIDKey)
	projectID := c.Param("id")

	// Verify project ownership
	p, err := h.projRepo.FindByIDAndOwner(c.Request.Context(), projectID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}
	if p == nil {
		c.JSON(http.StatusNotFound, apiErr("not_found", "project not found", reqID(c)))
		return
	}

	var statusFilter *Status
	if s := c.Query("status"); s != "" {
		st := Status(s)
		statusFilter = &st
	}
	var assigneeFilter *string
	if a := c.Query("assignee_id"); a != "" {
		assigneeFilter = &a
	}
	cursor := c.Query("cursor")
	limit := 20
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	tasks, nextCursor, err := h.repo.ListByProject(c.Request.Context(), projectID, statusFilter, assigneeFilter, cursor, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}

	var data []gin.H
	for _, t := range tasks {
		data = append(data, taskResp(t))
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
	t, err := h.repo.FindByIDForOwner(c.Request.Context(), id, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}
	if t == nil {
		c.JSON(http.StatusNotFound, apiErr("not_found", "task not found", reqID(c)))
		return
	}
	c.JSON(http.StatusOK, taskResp(t))
}

func (h *Handler) Update(c *gin.Context) {
	userID := c.GetString(middleware.UserIDKey)
	id := c.Param("id")

	// Verify ownership
	existing, err := h.repo.FindByIDForOwner(c.Request.Context(), id, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, apiErr("not_found", "task not found", reqID(c)))
		return
	}

	var req struct {
		Title      *string `json:"title"`
		Priority   *int    `json:"priority"`
		AssigneeID *string `json:"assignee_id"`
		DueDate    *string `json:"due_date"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, apiErr("validation_error", err.Error(), reqID(c)))
		return
	}

	fields := UpdateFields{
		Title:      req.Title,
		Priority:   req.Priority,
		AssigneeID: req.AssigneeID,
		DueDate:    req.DueDate,
	}
	t, err := h.repo.Update(c.Request.Context(), id, fields)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}
	if t == nil {
		c.JSON(http.StatusNotFound, apiErr("not_found", "task not found", reqID(c)))
		return
	}

	// Notify if assignee changed
	if req.AssigneeID != nil && h.notifier != nil {
		h.notifier.Publish(t.ID, *req.AssigneeID)
	}

	c.JSON(http.StatusOK, taskResp(t))
}

func (h *Handler) Delete(c *gin.Context) {
	userID := c.GetString(middleware.UserIDKey)
	id := c.Param("id")

	existing, err := h.repo.FindByIDForOwner(c.Request.Context(), id, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, apiErr("not_found", "task not found", reqID(c)))
		return
	}

	if err := h.repo.SoftDelete(c.Request.Context(), id, userID); err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) Transition(c *gin.Context) {
	userID := c.GetString(middleware.UserIDKey)
	id := c.Param("id")

	existing, err := h.repo.FindByIDForOwner(c.Request.Context(), id, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}
	if existing == nil {
		c.JSON(http.StatusNotFound, apiErr("not_found", "task not found", reqID(c)))
		return
	}

	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, apiErr("validation_error", err.Error(), reqID(c)))
		return
	}

	newStatus := Status(req.Status)
	if err := ValidateTransition(existing.Status, newStatus); err != nil {
		code := "task_invalid_transition"
		if strings.Contains(err.Error(), "task_invalid_transition") {
			code = "task_invalid_transition"
		}
		c.JSON(http.StatusConflict, apiErr(code, err.Error(), reqID(c)))
		return
	}

	fields := UpdateFields{Status: &newStatus}
	t, err := h.repo.Update(c.Request.Context(), id, fields)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}
	c.JSON(http.StatusOK, taskResp(t))
}

func taskResp(t *Task) gin.H {
	return gin.H{
		"id":          t.ID,
		"project_id":  t.ProjectID,
		"title":       t.Title,
		"status":      t.Status,
		"priority":    t.Priority,
		"assignee_id": t.AssigneeID,
		"due_date":    t.DueDate,
		"created_at":  t.CreatedAt,
		"updated_at":  t.UpdatedAt,
	}
}

func reqID(c *gin.Context) string { return c.GetString(middleware.RequestIDKey) }

func apiErr(code, message, requestID string) gin.H {
	return gin.H{"error": gin.H{"code": code, "message": message, "request_id": requestID}}
}

// contextKey is used for context values.
type contextKey struct{ name string }

// Ensure context.Context is imported
var _ = context.Background
