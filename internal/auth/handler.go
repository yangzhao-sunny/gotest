package auth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/test/taskmgr/internal/middleware"
)

type Handler struct{ svc *Service }

func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) Register(c *gin.Context) {
	var req struct {
		Email       string `json:"email" binding:"required,email"`
		Password    string `json:"password" binding:"required,min=8"`
		DisplayName string `json:"display_name" binding:"required,min=1,max=64"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, apiErr("validation_error", err.Error(), reqID(c)))
		return
	}
	u, err := h.svc.Register(c.Request.Context(), req.Email, req.Password, req.DisplayName)
	if err != nil {
		if errors.Is(err, ErrEmailTaken) {
			c.JSON(http.StatusConflict, apiErr("email_taken", "email already registered", reqID(c)))
		} else {
			c.JSON(http.StatusInternalServerError, apiErr("internal_error", "registration failed", reqID(c)))
		}
		return
	}
	c.JSON(http.StatusCreated, userResp(u))
}

func (h *Handler) Login(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, apiErr("validation_error", err.Error(), reqID(c)))
		return
	}
	pair, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, apiErr("invalid_credentials", "invalid email or password", reqID(c)))
		return
	}
	c.JSON(http.StatusOK, gin.H{"access_token": pair.AccessToken, "refresh_token": pair.RefreshToken})
}

func (h *Handler) Refresh(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, apiErr("validation_error", err.Error(), reqID(c)))
		return
	}
	access, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, apiErr("invalid_refresh_token", err.Error(), reqID(c)))
		return
	}
	c.JSON(http.StatusOK, gin.H{"access_token": access})
}

func (h *Handler) Logout(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, apiErr("validation_error", err.Error(), reqID(c)))
		return
	}
	_ = h.svc.Logout(c.Request.Context(), req.RefreshToken)
	c.Status(http.StatusNoContent)
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
