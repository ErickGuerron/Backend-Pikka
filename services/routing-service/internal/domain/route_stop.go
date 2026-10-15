package domain

// RouteStop es una parada de una ruta. Sequence es 1-based dentro de la ruta.
type RouteStop struct {
	OrderID  string
	ZoneID   string
	Sequence int
}
