package geocoding

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

func TestLocateReturnsFirstFeatureAsLatitudeLongitude(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"features":[{"geometry":{"type":"Point","coordinates":[-78.4678,-0.1807]}}]}`))
	}))
	defer server.Close()

	p := NewPhoton(server.URL, "-92.1,-5.1,-75.1,1.7", time.Second)
	lat, lng, found, err := p.Locate(context.Background(), "Avenida Amazonas 123")

	require.NoError(t, err)
	require.True(t, found)
	// GeoJSON entrega [longitud, latitud]: aquí se invierte al orden latitud, longitud.
	assert.InDelta(t, -0.1807, lat, 1e-9)
	assert.InDelta(t, -78.4678, lng, 1e-9)
	assert.Contains(t, gotQuery, "bbox=-92.1")
	assert.Contains(t, gotQuery, "limit=1")
}

func TestLocateWithoutFeaturesIsNotFoundNotAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"features":[]}`))
	}))
	defer server.Close()

	_, _, found, err := NewPhoton(server.URL, "", time.Second).Locate(context.Background(), "Calle inexistente")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestLocateProviderErrorIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	_, _, _, err := NewPhoton(server.URL, "", time.Second).Locate(context.Background(), "Avenida Amazonas 123")
	assert.True(t, errors.Is(err, domain.ErrGeocoderUnavailable), "err = %v", err)
}

func TestLocateTimeoutIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	_, _, _, err := NewPhoton(server.URL, "", 20*time.Millisecond).Locate(context.Background(), "Avenida Amazonas 123")
	assert.True(t, errors.Is(err, domain.ErrGeocoderUnavailable), "err = %v", err)
}
