// Package ports define las dependencias que la capa de aplicación necesita del exterior.
package ports

import (
	"context"
	"errors"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

var (
	// ErrDriverNotFound indica que no existe un repartidor con el identificador pedido.
	ErrDriverNotFound = errors.New("driver not found")
	// ErrDriverExists indica que ya existe un repartidor con ese identificador.
	ErrDriverExists = errors.New("driver already exists")
	// ErrVersionConflict indica que el repartidor cambió desde que se leyó. El llamador
	// debe volver a leerlo y decidir de nuevo.
	ErrVersionConflict = errors.New("driver version conflict")
)

// DriverRepository persiste repartidores. Save es un control optimista: solo guarda si la
// versión almacenada coincide con expectedVersion.
type DriverRepository interface {
	Create(ctx context.Context, d *domain.Driver) error
	Get(ctx context.Context, id string) (*domain.Driver, error)
	Save(ctx context.Context, d *domain.Driver, expectedVersion int) error
	// FindAvailable devuelve hasta limit repartidores activos y AVAILABLE, en orden de ID.
	FindAvailable(ctx context.Context, limit int) ([]*domain.Driver, error)
}
