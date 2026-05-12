package user

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/test/taskmgr/internal/middleware"
)

type Handler struct{ repo *Repo }

func NewHandler(repo *Repo) *Handler { return &Handler{repo: repo} }

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/users/me", h.GetMe)
	rg.PATCH("/users/me", h.PatchMe)
}

func (h *Handler) GetMe(c *gin.Context) {
	userID := c.GetString(middleware.UserIDKey)
	u, err := h.repo.FindByID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}
	if u == nil {
		c.JSON(http.StatusNotFound, apiErr("not_found", "user not found", reqID(c)))
		return
	}
	c.JSON(http.StatusOK, userResp(u))
}

func (h *Handler) PatchMe(c *gin.Context) {
	var req struct {
		DisplayName string `json:"display_name" binding:"required,min=1,max=64"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, apiErr("validation_error", err.Error(), reqID(c)))
		return
	}
	userID := c.GetString(middleware.UserIDKey)
	u, err := h.repo.UpdateDisplayName(c.Request.Context(), userID, req.DisplayName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErr("internal_error", err.Error(), reqID(c)))
		return
	}
	if u == nil {
		c.JSON(http.StatusNotFound, apiErr("not_found", "user not found", reqID(c)))
		return
	}
	c.JSON(http.StatusOK, userResp(u))
}

func userResp(u *User) gin.H {
	return gin.H{
		"id":           u.ID,
		"email":        u.Email,
		"display_name": u.DisplayName,
		"created_at":   u.CreatedAt,
	}
}

func reqID(c *gin.Context) string { return c.GetString(middleware.RequestIDKey) }

func apiErr(code, message, requestID string) gin.H {
	return gin.H{"error": gin.H{"code": code, "message": message, "request_id": requestID}}
}
