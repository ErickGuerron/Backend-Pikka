package http_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adapterhttp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/http"
)

func serve(t *testing.T, ping func(context.Context) error, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	adapterhttp.Handler(ping).ServeHTTP(rec, req)
	return rec
}

func TestHealthReportsTheProcessAlive(t *testing.T) {
	rec := serve(t, nil, http.MethodGet, "/health")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestReadyReportsReadyWhenTheDatabaseAnswers(t *testing.T) {
	var deadlineSet bool
	ping := func(ctx context.Context) error {
		_, deadlineSet = ctx.Deadline()
		return nil
	}

	rec := serve(t, ping, http.MethodGet, "/ready")

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"ready"}`, rec.Body.String())
	assert.True(t, deadlineSet, "la comprobación de disponibilidad tiene un plazo")
}

func TestReadyReportsUnavailableWhenThePingFails(t *testing.T) {
	ping := func(context.Context) error { return errors.New("connection refused") }

	rec := serve(t, ping, http.MethodGet, "/ready")

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.JSONEq(t, `{"status":"unavailable"}`, rec.Body.String())
}

func TestHealthRejectsOtherMethods(t *testing.T) {
	rec := serve(t, nil, http.MethodPost, "/health")

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}
