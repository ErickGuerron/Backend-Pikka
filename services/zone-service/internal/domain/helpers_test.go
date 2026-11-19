package domain_test

import (
	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/domain"
)

// validSettings es una configuración operativa completa y válida para las pruebas.
func validSettings() domain.Settings {
	return domain.Settings{
		Description:        "Cobertura de prueba",
		DepotName:          "Hub Norte",
		DepotLatitude:      -0.179,
		DepotLongitude:     -78.483,
		MaxOrdersPerRoute:  18,
		RouteCapacityKg:    45,
		Timezone:           "America/Guayaquil",
		ServiceDays:        []string{"MON", "TUE", "WED", "THU", "FRI", "SAT"},
		DeliveryWindowFrom: "08:00",
		DeliveryWindowTo:   "20:00",
		CutoffTime:         "14:00",
		DispatchTime:       "15:30",
		Priority:           domain.PriorityHigh,
		DailyOrderLimit:    250,
	}
}

// newZone crea una zona sin huecos con la configuración válida de las pruebas.
func newZone(code, name string, ring []domain.Coordinate) (domain.Zone, error) {
	return domain.NewZoneWithHoles(code, name, ring, nil, validSettings())
}

// newZoneWithHoles crea una zona con huecos y la configuración válida de las pruebas.
func newZoneWithHoles(code, name string, ring []domain.Coordinate, holes [][]domain.Coordinate) (domain.Zone, error) {
	return domain.NewZoneWithHoles(code, name, ring, holes, validSettings())
}
