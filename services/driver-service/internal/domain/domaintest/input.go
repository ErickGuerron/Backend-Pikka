// Package domaintest ofrece datos válidos de repartidor para las pruebas de otros paquetes.
package domaintest

import (
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

// Input devuelve una entrada válida para un repartidor nuevo. Identificador y placa salen
// del userID para que dos repartidores de una misma prueba no choquen en las restricciones únicas.
func Input(userID string) domain.NewDriverInput {
	return domain.NewDriverInput{
		UserID:      userID,
		FirstName:   "Ana",
		LastName:    "Ruiz",
		BirthDate:   time.Date(1995, 5, 20, 0, 0, 0, 0, time.UTC),
		Phone:       "0991234567",
		VehicleType: "MOTORIZADO",
		Identifier:  "ID" + sanitize(userID),
		Plate:       "PL" + sanitize(userID),
		City:        "Quito",
	}
}

// sanitize toma los últimos 5 caracteres alfanuméricos: los ids de las pruebas difieren al final ("user-d1", "user-d2").
func sanitize(s string) string {
	var out []byte
	for i := len(s) - 1; i >= 0 && len(out) < 5; i-- {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			out = append([]byte{c}, out...)
		}
	}
	for len(out) < 3 {
		out = append(out, '0')
	}
	return string(out)
}
