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
	// ErrStopNotFound indica que la fuente de paradas no conoce la parada de esa ruta.
	ErrStopNotFound = errors.New("stop not found")
)

// DriverRepository persiste repartidores. Save es un control optimista: solo guarda si la
// versión almacenada coincide con expectedVersion.
type DriverRepository interface {
	Create(ctx context.Context, d *domain.Driver) error
	Get(ctx context.Context, id string) (*domain.Driver, error)
	// FindByUserID devuelve el repartidor cuyo user_id es userID, o ErrDriverNotFound.
	FindByUserID(ctx context.Context, userID string) (*domain.Driver, error)
	Save(ctx context.Context, d *domain.Driver, expectedVersion int) error
	// FindAvailable devuelve hasta limit repartidores activos y AVAILABLE, en orden de ID.
	FindAvailable(ctx context.Context, limit int) ([]*domain.Driver, error)
	// List devuelve hasta limit repartidores, activos o no, ordenados por apellido, nombre e ID.
	List(ctx context.Context, limit int) ([]*domain.Driver, error)
}

// ErrDeliveryAlreadyRecorded indica que la parada ya tiene un registro de entrega en esa ruta.
var ErrDeliveryAlreadyRecorded = errors.New("delivery already recorded for this stop")

// DeliveryRecorder guarda el registro de entrega de una parada. Record rechaza un segundo
// registro de la misma parada dentro de la misma ruta.
type DeliveryRecorder interface {
	Record(ctx context.Context, routeID string, rec domain.DeliveryRecord, arrival domain.ArrivalCheck) error
}
