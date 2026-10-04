package http_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	httpadapter "github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/adapters/http"
)

func TestHealthEndpoints(t *testing.T) {
	ok := httpadapter.NewHealthServer(func(context.Context) error { return nil })
	down := httpadapter.NewHealthServer(func(context.Context) error { return errors.New("db down") })

	for _, tc := range []struct {
		srv  http.Handler
		path string
		want int
	}{
		{ok, "/health", http.StatusOK},
		{ok, "/ready", http.StatusOK},
		{down, "/health", http.StatusOK},
		{down, "/ready", http.StatusServiceUnavailable},
	} {
		rec := httptest.NewRecorder()
		tc.srv.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		assert.Equal(t, tc.want, rec.Code, tc.path)
	}
}
