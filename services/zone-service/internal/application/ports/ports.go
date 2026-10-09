// Package ports define las dependencias que la capa de aplicación necesita del
// exterior. Los adaptadores (PostgreSQL, gRPC) las implementan.
package ports

import (
	"context"

	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/domain"
)

// ZoneRepository persiste zonas y responde consultas geográficas.
type ZoneRepository interface {
	// Save inserta una zona nueva y devuelve la versión persistida (con ID y fecha).
	// Devuelve domain.ErrZoneCodeTaken si el código ya existe.
	Save(ctx context.Context, zone domain.Zone) (domain.Zone, error)

	// FindByID devuelve la zona o domain.ErrZoneNotFound.
	FindByID(ctx context.Context, id string) (domain.Zone, error)

	// FindContaining devuelve la zona que contiene el punto (la de menor área si
	// hay solapamiento) o domain.ErrZoneNotFound.
	FindContaining(ctx context.Context, point domain.Coordinate) (domain.Zone, error)
}
