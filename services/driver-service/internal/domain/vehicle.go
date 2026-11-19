package domain

import "strings"

// VehicleType es el tipo de vehículo con el que el repartidor hace las entregas.
type VehicleType string

const (
	VehicleMotorbike VehicleType = "MOTORIZADO"
	VehicleCar       VehicleType = "AUTOMOVIL"
	VehicleSUV       VehicleType = "SUV"
)

// ParseVehicleType normaliza el texto (mayúsculas, sin espacios) y comprueba que sea un tipo conocido.
func ParseVehicleType(raw string) (VehicleType, error) {
	v := VehicleType(strings.ToUpper(strings.TrimSpace(raw)))
	switch v {
	case VehicleMotorbike, VehicleCar, VehicleSUV:
		return v, nil
	default:
		return "", ErrInvalidVehicleType
	}
}
