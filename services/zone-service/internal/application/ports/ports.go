// Package ports define las dependencias que la capa de aplicación necesita del
// exterior. Los adaptadores (PostgreSQL, gRPC) las implementan.
package ports

import (
	"context"

	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/domain"
)

// ZoneRepository persiste zonas y responde consultas geográficas. Los cambios registran
// la bitácora con el usuario que viene en el contexto (domain.ActorFrom).
type ZoneRepository interface {
	// Save inserta una zona nueva y devuelve la versión persistida (con ID y fecha).
	// Devuelve domain.ErrZoneCodeTaken si el código ya existe.
	Save(ctx context.Context, zone domain.Zone) (domain.Zone, error)

	// Update reemplaza nombre y geometría de una zona existente (el código no cambia).
	// Devuelve domain.ErrZoneNotFound si no existe.
	Update(ctx context.Context, zone domain.Zone) (domain.Zone, error)

	// SetStatus cambia el estado y guarda el motivo. Devuelve domain.ErrZoneNotFound si no existe.
	SetStatus(ctx context.Context, id, status, reason string) (domain.Zone, error)

	// Delete borra físicamente una zona inactiva. Devuelve domain.ErrZoneNotFound si no existe y
	// domain.ErrZoneNotInactive si todavía está activa.
	Delete(ctx context.Context, id, reason string) error

	// NextCode reserva el siguiente número para un prefijo de código (1, 2, 3...). Es atómico.
	NextCode(ctx context.Context, prefix string) (int, error)

	// FindByID devuelve la zona, activa o no, o domain.ErrZoneNotFound.
	FindByID(ctx context.Context, id string) (domain.Zone, error)

	// ListAll devuelve todas las zonas, activas o no, ordenadas por código.
	ListAll(ctx context.Context) ([]domain.Zone, error)

	// FindContaining devuelve la zona activa que contiene el punto (la de menor área si hay
	// solapamiento) o domain.ErrZoneNotFound. Las zonas inactivas no cuentan.
	FindContaining(ctx context.Context, point domain.Coordinate) (domain.Zone, error)
}
