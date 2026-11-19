//go:build integration

package osrm

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// Requiere OSRM_URL apuntando a un osrm-routed con el mapa de Ecuador cargado.
// Ejecutar con: go test -tags integration ./...  (desde libs/osrm)
func integrationEstimator(t *testing.T) *Estimator {
	t.Helper()
	base := os.Getenv("OSRM_URL")
	if base == "" {
		t.Skip("OSRM_URL not set")
	}
	return NewEstimator(base, 10*time.Second)
}

// Dos puntos del centro de Quito: distancia y tiempo realistas para una ciudad.
func TestOSRMEstimatesQuitoRoute(t *testing.T) {
	est := integrationEstimator(t)

	got, err := est.Estimate(context.Background(), quito, quitoSouth)
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if got.DistanceMeters < 3000 || got.DistanceMeters > 15000 {
		t.Fatalf("distance = %.0f m, expected a few km inside Quito", got.DistanceMeters)
	}
	if got.DurationSeconds <= 0 || got.DurationSeconds > 3600 {
		t.Fatalf("duration = %.0f s, expected under an hour inside Quito", got.DurationSeconds)
	}
}

// Bogotá está fuera del mapa de Ecuador: debe responder "sin ruta", no un error de servidor.
func TestOSRMOutsideEcuadorIsNoRoute(t *testing.T) {
	est := integrationEstimator(t)
	bogota := Point{Latitude: 4.711, Longitude: -74.072}

	_, err := est.Estimate(context.Background(), quito, bogota)
	if !errors.Is(err, ErrNoRoute) {
		t.Fatalf("err = %v, want ErrNoRoute for a point outside the loaded map", err)
	}
}
