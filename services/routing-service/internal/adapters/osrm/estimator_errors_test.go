package osrm

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

// Un fallo del servidor OSRM que no es "sin ruta" ni "punto inválido" pasa tal cual al dominio.
func TestUnexpectedOSRMFailurePassesThroughUnchanged(t *testing.T) {
	srv := newServer(t, http.StatusInternalServerError, `{"code":"Error"}`)
	est := NewEstimator(srv.URL, 2*time.Second)

	_, err := est.Estimate(context.Background(), quito, quitoSouth)

	if err == nil {
		t.Fatal("expected an error when OSRM fails")
	}
	if errors.Is(err, domain.ErrNoRoute) || errors.Is(err, domain.ErrInvalidPoint) {
		t.Fatalf("an infrastructure failure must not look like a routing answer: %v", err)
	}
}
