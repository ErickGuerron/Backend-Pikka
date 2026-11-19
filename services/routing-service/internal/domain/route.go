package domain

import "fmt"

// Route es un grupo ordenado de paradas de una sola zona.
// Su ID es una clave de planificacion determinista, no un identificador persistente.
type Route struct {
	ID     string
	ZoneID string
	Stops  []RouteStop
}

// routeID construye el ID "{zone_id}#{n}" con n 1-based por zona.
func routeID(zoneID string, n int) string {
	return fmt.Sprintf("%s#%d", zoneID, n)
}
