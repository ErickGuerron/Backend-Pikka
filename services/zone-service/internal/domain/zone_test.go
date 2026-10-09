package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/domain"
)

func square(t *testing.T) []domain.Coordinate {
	t.Helper()
	pts := [][2]float64{{-0.20, -78.50}, {-0.20, -78.40}, {-0.10, -78.40}, {-0.10, -78.50}, {-0.20, -78.50}}
	ring := make([]domain.Coordinate, 0, len(pts))
	for _, p := range pts {
		c, err := domain.NewCoordinate(p[0], p[1])
		require.NoError(t, err)
		ring = append(ring, c)
	}
	return ring
}

func TestNewZone_ValidCreatesZone(t *testing.T) {
	zone, err := domain.NewZone("  NORTE  ", " Zona Norte ", square(t))

	require.NoError(t, err)
	assert.Equal(t, "NORTE", zone.Code)
	assert.Equal(t, "Zona Norte", zone.Name)
	assert.Len(t, zone.Ring, 5)
}

func TestNewZone_RejectsInvalidInput(t *testing.T) {
	ring := square(t)
	open := ring[:4] // sin repetir el primer punto al final

	tests := []struct {
		name    string
		code    string
		zone    string
		ring    []domain.Coordinate
		wantErr error
	}{
		{"empty code", "", "Norte", ring, domain.ErrInvalidCode},
		{"code too long", string(make([]byte, 33)), "Norte", ring, domain.ErrInvalidCode},
		{"empty name", "N", "   ", ring, domain.ErrInvalidName},
		{"too few points", "N", "Norte", ring[:3], domain.ErrInvalidRing},
		{"open ring", "N", "Norte", open, domain.ErrRingNotClosed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewZone(tt.code, tt.zone, tt.ring)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestNewCoordinate_RejectsOutOfRange(t *testing.T) {
	_, err := domain.NewCoordinate(91, 0)
	assert.ErrorIs(t, err, domain.ErrInvalidCoordinate)

	_, err = domain.NewCoordinate(0, -181)
	assert.ErrorIs(t, err, domain.ErrInvalidCoordinate)
}
