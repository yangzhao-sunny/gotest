//go:build integration

package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/test/taskmgr/internal/testhelper"
)

func newHealthRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v1 := r.Group("/v1")
	h.RegisterRoutes(v1)
	return r
}

func Test_Readyz_BothHealthy_Integration(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewPool(t, ctx)
	redis := testhelper.NewRedis(t, ctx)

	h := NewHandler(pool, redis)
	r := newHealthRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/v1/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func Test_Readyz_PoolClosed_Returns503_Integration(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewPool(t, ctx)
	redis := testhelper.NewRedis(t, ctx)

	h := NewHandler(pool, redis)
	r := newHealthRouter(h)

	// Verify it's healthy first
	req := httptest.NewRequest(http.MethodGet, "/v1/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Close the pool
	pool.Close()

	// Now readyz should return 503
	req = httptest.NewRequest(http.MethodGet, "/v1/readyz", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
