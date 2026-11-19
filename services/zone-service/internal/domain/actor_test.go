package domain_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/domain"
)

func TestActor_RoundTripsThroughContext(t *testing.T) {
	ctx := domain.WithActor(context.Background(), "user-42")

	assert.Equal(t, "user-42", domain.ActorFrom(ctx))
}

func TestActor_EmptyWhenNotSet(t *testing.T) {
	assert.Empty(t, domain.ActorFrom(context.Background()))
}

func TestValidStatus(t *testing.T) {
	tests := map[string]bool{
		domain.StatusActive:   true,
		domain.StatusInactive: true,
		"PAUSED":              false,
		"active":              false,
		"":                    false,
	}
	for status, want := range tests {
		t.Run(status, func(t *testing.T) {
			assert.Equal(t, want, domain.ValidStatus(status))
		})
	}
}

func TestNewZone_RingWithTooFewPointsIsInvalid(t *testing.T) {
	// Un anillo de solo cuatro puntos no cumple el mínimo de cinco (cuatro vértices más el cierre).
	pts := ring(t, [2]float64{0, 0}, [2]float64{0, 4}, [2]float64{4, 4}, [2]float64{4, 0})

	_, err := newZone("C", "Zona", pts)

	assert.ErrorIs(t, err, domain.ErrInvalidRing)
}

func TestNewZone_RingWithInvalidCoordinateIsRejected(t *testing.T) {
	// Latitud fuera de rango: el punto de cierre es igual al primero, así que llega a la validación de coordenadas.
	pts := []domain.Coordinate{{Latitude: 0, Longitude: 0}, {Latitude: 0, Longitude: 4}, {Latitude: 4, Longitude: 4}, {Latitude: 95, Longitude: 0}, {Latitude: 0, Longitude: 0}}

	_, err := newZone("C", "Zona", pts)

	assert.ErrorIs(t, err, domain.ErrInvalidCoordinate)
}
