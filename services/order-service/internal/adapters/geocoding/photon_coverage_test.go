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

func TestLocateSkipsFeaturesWithoutAPointAndUsesTheNextOne(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"features":[{"geometry":{"coordinates":[-78.4]}},{"geometry":{"coordinates":[-78.5,-0.2]}}]}`))
	}))
	defer server.Close()

	lat, lng, found, err := NewPhoton(server.URL, "", time.Second).Locate(context.Background(), "Calle sin punto")

	require.NoError(t, err)
	require.True(t, found)
	assert.InDelta(t, -0.2, lat, 1e-9)
	assert.InDelta(t, -78.5, lng, 1e-9)
}

func TestLocateFeatureWithoutCoordinatesIsNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"features":[{"geometry":{"coordinates":[]}}]}`))
	}))
	defer server.Close()

	_, _, found, err := NewPhoton(server.URL, "", time.Second).Locate(context.Background(), "Calle vacía")

	require.NoError(t, err)
	assert.False(t, found)
}

func TestLocateMalformedJSONIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"features":`))
	}))
	defer server.Close()

	_, _, _, err := NewPhoton(server.URL, "", time.Second).Locate(context.Background(), "Avenida Amazonas 123")

	assert.True(t, errors.Is(err, domain.ErrGeocoderUnavailable), "err = %v", err)
}

func TestLocateInvalidBaseURLFailsBeforeCallingProvider(t *testing.T) {
	// Un carácter de control en la URL impide construir la petición.
	_, _, found, err := NewPhoton("http://[::1\x7f", "", time.Second).Locate(context.Background(), "Avenida Amazonas 123")

	assert.Error(t, err)
	assert.False(t, found)
	assert.False(t, errors.Is(err, domain.ErrGeocoderUnavailable), "un error de configuración no es un proveedor caído")
}

func TestLocateUnreachableProviderIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close() // el puerto ya no escucha

	_, _, _, err := NewPhoton(server.URL, "", time.Second).Locate(context.Background(), "Avenida Amazonas 123")

	assert.True(t, errors.Is(err, domain.ErrGeocoderUnavailable), "err = %v", err)
}
