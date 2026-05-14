package health

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func Test_Healthz_AlwaysOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHandler(nil, nil)

	r := gin.New()
	v1 := r.Group("/v1")
	h.RegisterRoutes(v1)

	req := httptest.NewRequest(http.MethodGet, "/v1/healthz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
