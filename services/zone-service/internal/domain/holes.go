package domain

// Geometría de huecos. El exterior y cada hueco son polígonos convexos (ver shape.go),
// así que las comprobaciones se reducen a signos de productos cruzados y a proyecciones
// sobre ejes: exactas y sin depender de parámetros.

// signedArea devuelve el doble del área con signo: positivo si el anillo va en sentido antihorario.
func signedArea(vertices []Coordinate) float64 {
	var sum float64
	n := len(vertices)
	for i := range n {
		a := vertices[i]
		b := vertices[(i+1)%n]
		sum += a.Longitude*b.Latitude - b.Longitude*a.Latitude
	}
	return sum
}

// strictlyInsideConvex indica si p está en el interior estricto del polígono convexo
// (no en el borde ni fuera). Funciona con cualquier orientación.
func strictlyInsideConvex(p Coordinate, polygon []Coordinate) bool {
	n := len(polygon)
	if n < minVertices {
		return false
	}
	side := 1.0
	if signedArea(polygon) < 0 {
		side = -1
	}
	for i := range n {
		a := polygon[i]
		b := polygon[(i+1)%n]
		if side*cross(a, b, p) <= 0 {
			return false
		}
	}
	return true
}

// cross es el producto cruzado de oa y ob (ver geometría de shape.go).
func cross(o, a, b Coordinate) float64 {
	return (a.Longitude-o.Longitude)*(b.Latitude-o.Latitude) - (a.Latitude-o.Latitude)*(b.Longitude-o.Longitude)
}

// disjointConvex indica si dos polígonos convexos no se tocan ni se superponen.
// Teorema del eje separador: si existe una recta que los separa estrictamente, son disjuntos.
// Tocarse en un punto o en un lado cuenta como superponerse.
func disjointConvex(a, b []Coordinate) bool {
	return separatedByAxesOf(a, b) || separatedByAxesOf(b, a)
}

// separatedByAxesOf prueba los ejes perpendiculares a los lados de `owner`.
func separatedByAxesOf(owner, other []Coordinate) bool {
	n := len(owner)
	for i := range n {
		p := owner[i]
		q := owner[(i+1)%n]
		// Normal al lado pq: el eje de proyección.
		axisX := -(q.Latitude - p.Latitude)
		axisY := q.Longitude - p.Longitude
		minO, maxO := project(owner, axisX, axisY)
		minR, maxR := project(other, axisX, axisY)
		if maxO < minR || maxR < minO {
			return true
		}
	}
	return false
}

// project devuelve el intervalo de proyecciones del polígono sobre el eje (x, y).
func project(polygon []Coordinate, x, y float64) (float64, float64) {
	first := polygon[0]
	lo := first.Longitude*x + first.Latitude*y
	hi := lo
	for _, p := range polygon[1:] {
		v := p.Longitude*x + p.Latitude*y
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	return lo, hi
}

// validateHoles comprueba que cada hueco (ya validado como anillo convexo) quede dentro del
// exterior y que los huecos no se superpongan entre sí.
func validateHoles(outer []Coordinate, holes [][]Coordinate) error {
	for _, hole := range holes {
		for _, p := range hole[:len(hole)-1] {
			if !strictlyInsideConvex(p, outer[:len(outer)-1]) {
				return ErrHoleOutside
			}
		}
	}
	for i := range holes {
		for j := i + 1; j < len(holes); j++ {
			if !disjointConvex(holes[i][:len(holes[i])-1], holes[j][:len(holes[j])-1]) {
				return ErrHolesOverlap
			}
		}
	}
	return nil
}
