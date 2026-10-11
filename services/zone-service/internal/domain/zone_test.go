package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/domain"
)

// ring convierte pares {lat, lng} en un anillo validado. Cada prueba indica si
// el anillo debe cerrarse con su primer punto.
func ring(t *testing.T, pts ...[2]float64) []domain.Coordinate {
	t.Helper()
	out := make([]domain.Coordinate, 0, len(pts))
	for _, p := range pts {
		c, err := domain.NewCoordinate(p[0], p[1])
		require.NoError(t, err)
		out = append(out, c)
	}
	return out
}

// closed agrega el primer punto al final, como exige zones.v1.
func closed(t *testing.T, pts ...[2]float64) []domain.Coordinate {
	t.Helper()
	return ring(t, append(pts, pts[0])...)
}

func square(t *testing.T) []domain.Coordinate {
	t.Helper()
	return closed(t, [2]float64{-0.20, -78.50}, [2]float64{-0.20, -78.40}, [2]float64{-0.10, -78.40}, [2]float64{-0.10, -78.50})
}

func TestNewZone_ValidCreatesZone(t *testing.T) {
	zone, err := domain.NewZone("  NORTE  ", " Zona Norte ", square(t))

	require.NoError(t, err)
	assert.Equal(t, "NORTE", zone.Code)
	assert.Equal(t, "Zona Norte", zone.Name)
	assert.Len(t, zone.Ring, 5)
}

func TestNewZone_RejectsInvalidInput(t *testing.T) {
	sq := square(t)
	// Cinco puntos que no cierran: el último no coincide con el primero.
	open := []domain.Coordinate{sq[0], sq[1], sq[2], sq[3], sq[3]}

	tests := []struct {
		name    string
		code    string
		zone    string
		ring    []domain.Coordinate
		wantErr error
	}{
		{"empty code", "", "Norte", sq, domain.ErrInvalidCode},
		{"code too long", string(make([]byte, 33)), "Norte", sq, domain.ErrInvalidCode},
		{"empty name", "N", "   ", sq, domain.ErrInvalidName},
		{"too few points", "N", "Norte", sq[:3], domain.ErrInvalidRing},
		{"open ring", "N", "Norte", open, domain.ErrRingNotClosed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := domain.NewZone(tt.code, tt.zone, tt.ring)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestNewZone_AcceptsConvexQuadrilateralsAndPolygons(t *testing.T) {
	shapes := map[string][]domain.Coordinate{
		"cuadrado": square(t),
		"trapecio": closed(t, [2]float64{0, 0}, [2]float64{0, 4}, [2]float64{1, 3}, [2]float64{1, 1}),
		"rombo":    closed(t, [2]float64{0, 2}, [2]float64{1, 0}, [2]float64{2, 2}, [2]float64{1, 4}),
		"hexágono": closed(t, [2]float64{0, 1}, [2]float64{0, 3}, [2]float64{1, 4}, [2]float64{2, 3}, [2]float64{2, 1}, [2]float64{1, 0}),
	}
	for name, r := range shapes {
		t.Run(name, func(t *testing.T) {
			_, err := domain.NewZone("C", "Zona", r)
			assert.NoError(t, err)
		})
	}
}

func TestNewZone_RejectsIrregularShapes(t *testing.T) {
	tests := map[string][]domain.Coordinate{
		// Triángulo: solo 3 vértices.
		"triángulo": closed(t, [2]float64{0, 0}, [2]float64{0, 4}, [2]float64{3, 0}),
		// Cuadrilátero con una muesca hacia adentro (dardo).
		"muesca hacia adentro": closed(t, [2]float64{0, 0}, [2]float64{4, 2}, [2]float64{0, 4}, [2]float64{1, 2}),
		// Dos lados que se cruzan (mariposa).
		"lados cruzados": closed(t, [2]float64{0, 0}, [2]float64{2, 2}, [2]float64{2, 0}, [2]float64{0, 2}),
		// Estrella de cinco puntas: giros del mismo lado, pero dan dos vueltas.
		"estrella": closed(t,
			[2]float64{1, 0}, [2]float64{-0.809, 0.588}, [2]float64{0.309, -0.951}, [2]float64{0.309, 0.951}, [2]float64{-0.809, -0.588}),
		// Vértice sobre una recta: no es una esquina.
		"vértice plano": closed(t, [2]float64{0, 0}, [2]float64{0, 2}, [2]float64{0, 4}, [2]float64{4, 4}, [2]float64{4, 0}),
	}
	for name, r := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := domain.NewZone("C", "Zona", r)
			assert.Error(t, err)
		})
	}
}

func TestNewZone_IrregularShapeIsNotConvex(t *testing.T) {
	_, err := domain.NewZone("C", "Zona", closed(t, [2]float64{0, 0}, [2]float64{4, 2}, [2]float64{0, 4}, [2]float64{1, 2}))
	assert.ErrorIs(t, err, domain.ErrNotConvex)
}

// donut es un cuadrado exterior de 4x4 con un hueco centrado de 2x2.
func donutOuter(t *testing.T) []domain.Coordinate {
	t.Helper()
	return closed(t, [2]float64{0, 0}, [2]float64{0, 4}, [2]float64{4, 4}, [2]float64{4, 0})
}

func TestNewZoneWithHoles_AcceptsHoleStrictlyInside(t *testing.T) {
	hole := closed(t, [2]float64{1, 1}, [2]float64{1, 3}, [2]float64{3, 3}, [2]float64{3, 1})
	zone, err := domain.NewZoneWithHoles("C", "Dona", donutOuter(t), [][]domain.Coordinate{hole})

	require.NoError(t, err)
	assert.Len(t, zone.Holes, 1)
	assert.Equal(t, hole, zone.Holes[0])
}

func TestNewZoneWithHoles_RejectsHoleOutsideOrTouchingOuter(t *testing.T) {
	tests := map[string][]domain.Coordinate{
		"fuera del exterior": closed(t, [2]float64{5, 5}, [2]float64{5, 7}, [2]float64{7, 7}, [2]float64{7, 5}),
		// Toca el borde del exterior: no es estrictamente interior.
		"toca el borde": closed(t, [2]float64{0, 1}, [2]float64{0, 3}, [2]float64{2, 3}, [2]float64{2, 1}),
	}
	for name, hole := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := domain.NewZoneWithHoles("C", "Dona", donutOuter(t), [][]domain.Coordinate{hole})
			assert.ErrorIs(t, err, domain.ErrHoleOutside)
		})
	}
}

func TestNewZoneWithHoles_RejectsOverlappingOrTouchingHoles(t *testing.T) {
	left := closed(t, [2]float64{1, 0.5}, [2]float64{1, 1.5}, [2]float64{3, 1.5}, [2]float64{3, 0.5})
	overlapping := closed(t, [2]float64{1, 1}, [2]float64{1, 2}, [2]float64{2, 2}, [2]float64{2, 1})
	// Compartir un lado: se tocan, así que se consideran superpuestos.
	touching := closed(t, [2]float64{1, 1.5}, [2]float64{1, 2.5}, [2]float64{2, 2.5}, [2]float64{2, 1.5})

	for name, other := range map[string][]domain.Coordinate{"superpuestos": overlapping, "tocándose": touching} {
		t.Run(name, func(t *testing.T) {
			_, err := domain.NewZoneWithHoles("C", "Dona", donutOuter(t), [][]domain.Coordinate{left, other})
			assert.ErrorIs(t, err, domain.ErrHolesOverlap)
		})
	}
}

func TestNewZoneWithHoles_AcceptsSeparateHoles(t *testing.T) {
	a := closed(t, [2]float64{0.5, 0.5}, [2]float64{0.5, 1.5}, [2]float64{1.5, 1.5}, [2]float64{1.5, 0.5})
	b := closed(t, [2]float64{2.5, 2.5}, [2]float64{2.5, 3.5}, [2]float64{3.5, 3.5}, [2]float64{3.5, 2.5})
	zone, err := domain.NewZoneWithHoles("C", "Dona", donutOuter(t), [][]domain.Coordinate{a, b})

	require.NoError(t, err)
	assert.Len(t, zone.Holes, 2)
}

func TestNewZoneWithHoles_HoleMustBeAValidConvexRing(t *testing.T) {
	triangle := closed(t, [2]float64{1, 1}, [2]float64{1, 3}, [2]float64{2, 1})
	_, err := domain.NewZoneWithHoles("C", "Dona", donutOuter(t), [][]domain.Coordinate{triangle})
	assert.ErrorIs(t, err, domain.ErrInvalidRing)
}

func TestNewCoordinate_RejectsOutOfRange(t *testing.T) {
	_, err := domain.NewCoordinate(91, 0)
	assert.ErrorIs(t, err, domain.ErrInvalidCoordinate)

	_, err = domain.NewCoordinate(0, -181)
	assert.ErrorIs(t, err, domain.ErrInvalidCoordinate)
}
