package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const UserIDKey = "user_id"

func Auth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, errResp("unauthorized", "missing bearer token", c.GetString(RequestIDKey)))
			return
		}
		tokenStr := strings.TrimPrefix(header, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(secret), nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, errResp("unauthorized", "invalid or expired token", c.GetString(RequestIDKey)))
			return
		}
		claims, _ := token.Claims.(jwt.MapClaims)
		c.Set(UserIDKey, claims["sub"])
		c.Next()
	}
}

func errResp(code, message, reqID string) gin.H {
	return gin.H{"error": gin.H{"code": code, "message": message, "request_id": reqID}}
}
