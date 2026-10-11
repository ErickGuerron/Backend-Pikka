package domain

import "math"

// shapeAngleTolerance absorbe el error de punto flotante al sumar los giros.
const shapeAngleTolerance = 1e-6

// isConvexPolygon indica si los vértices (sin repetir el punto de cierre) forman un
// polígono convexo y simple: un trapecio, un cuadrado o una figura similar.
//
// Un polígono es convexo y simple si y solo si:
//   - cada esquina tiene un giro distinto de cero (no hay vértices planos ni repetidos),
//   - todos los giros van hacia el mismo lado, y
//   - la suma de los giros es exactamente una vuelta completa (2π).
//
// La última condición descarta lados que se cruzan y estrellas, que también tienen
// giros del mismo lado pero dan más de una vuelta.
func isConvexPolygon(vertices []Coordinate) bool {
	n := len(vertices)
	if n < minVertices {
		return false
	}

	var turnSide float64
	var totalTurn float64
	for i := range n {
		a := vertices[i]
		b := vertices[(i+1)%n]
		c := vertices[(i+2)%n]

		// Lados consecutivos. Longitud es x y latitud es y.
		e1x, e1y := b.Longitude-a.Longitude, b.Latitude-a.Latitude
		e2x, e2y := c.Longitude-b.Longitude, c.Latitude-b.Latitude

		cross := e1x*e2y - e1y*e2x
		if cross == 0 {
			return false
		}
		side := math.Copysign(1, cross)
		if turnSide == 0 {
			turnSide = side
		} else if side != turnSide {
			return false
		}
		totalTurn += math.Atan2(cross, e1x*e2x+e1y*e2y)
	}

	return math.Abs(math.Abs(totalTurn)-2*math.Pi) < shapeAngleTolerance
}
