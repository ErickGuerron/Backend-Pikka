// Package config lee la configuración del Driver Service desde variables de entorno.
package config

import (
	"fmt"
	"math"
	"os"
	"strconv"
)

// DefaultArrivalRadiusMeters es el radio de llegada por defecto. Es un valor propuesto,
// no definido en los documentos: se puede cambiar con DRIVER_ARRIVAL_RADIUS_METERS.
const DefaultArrivalRadiusMeters = 50

// Config agrupa la configuración del proceso.
type Config struct {
	// ArrivalRadiusMeters es la distancia máxima, en metros, entre el GPS del repartidor y
	// la dirección de la parada para que la llegada cuente como válida.
	ArrivalRadiusMeters float64
}

// Load construye la configuración desde el entorno. Un radio vacío usa el valor por
// defecto; un valor no numérico, cero, negativo o no finito falla.
func Load() (Config, error) {
	raw := os.Getenv("DRIVER_ARRIVAL_RADIUS_METERS")
	if raw == "" {
		return Config{ArrivalRadiusMeters: DefaultArrivalRadiusMeters}, nil
	}
	radius, err := ParseRadius(raw)
	if err != nil {
		return Config{}, err
	}
	return Config{ArrivalRadiusMeters: radius}, nil
}

// ParseRadius convierte el texto de un radio en metros y valida que sea positivo y finito.
func ParseRadius(raw string) (float64, error) {
	radius, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("DRIVER_ARRIVAL_RADIUS_METERS must be a number: %w", err)
	}
	if math.IsNaN(radius) || math.IsInf(radius, 0) || radius <= 0 {
		return 0, fmt.Errorf("DRIVER_ARRIVAL_RADIUS_METERS must be a positive number, got %q", raw)
	}
	return radius, nil
}
