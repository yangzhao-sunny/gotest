package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func init() { gin.SetMode(gin.TestMode) }

func makeToken(secret, sub string, exp time.Time) string {
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": sub,
		"exp": exp.Unix(),
	})
	s, _ := tok.SignedString([]byte(secret))
	return s
}

func TestAuth_MissingToken(t *testing.T) {
	r := gin.New()
	r.Use(Auth("secret"))
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/x", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestAuth_InvalidToken(t *testing.T) {
	r := gin.New()
	r.Use(Auth("secret"))
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer notavalidtoken")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestAuth_ExpiredToken(t *testing.T) {
	r := gin.New()
	r.Use(Auth("secret"))
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	tok := makeToken("secret", "user-1", time.Now().Add(-1*time.Hour))
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
}

func TestAuth_ValidToken(t *testing.T) {
	r := gin.New()
	r.Use(Auth("secret"))
	r.GET("/x", func(c *gin.Context) {
		uid, _ := c.Get(UserIDKey)
		c.JSON(200, gin.H{"uid": uid})
	})

	tok := makeToken("secret", "user-42", time.Now().Add(1*time.Hour))
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("want 200, got %d: %s", w.Code, w.Body.String())
	}
	if !contains(w.Body.String(), "user-42") {
		t.Fatalf("user_id not in response: %s", w.Body.String())
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
