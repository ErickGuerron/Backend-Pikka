// Package memory implementa ports.DriverRepository en memoria. Sirve para pruebas y
// desarrollo local; no reemplaza la persistencia con PostgreSQL.
package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

// DriverRepository guarda copias de los repartidores para que el llamador no los modifique
// por accidente antes de Save.
type DriverRepository struct {
	mu      sync.Mutex
	drivers map[string]domain.Driver
}

// NewDriverRepository devuelve un repositorio vacío.
func NewDriverRepository() *DriverRepository {
	return &DriverRepository{drivers: make(map[string]domain.Driver)}
}

// Create agrega un repartidor nuevo.
func (r *DriverRepository) Create(_ context.Context, d *domain.Driver) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.drivers[d.ID]; ok {
		return ports.ErrDriverExists
	}
	for _, other := range r.drivers {
		if other.UserID == d.UserID || other.Identifier == d.Identifier || other.Plate == d.Plate {
			return ports.ErrDriverExists
		}
	}
	r.drivers[d.ID] = *d
	return nil
}

// Get devuelve una copia del repartidor.
func (r *DriverRepository) Get(_ context.Context, id string) (*domain.Driver, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.drivers[id]
	if !ok {
		return nil, ports.ErrDriverNotFound
	}
	return &d, nil
}

// FindByUserID devuelve una copia del repartidor cuyo user_id coincide.
func (r *DriverRepository) FindByUserID(_ context.Context, userID string) (*domain.Driver, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, d := range r.drivers {
		if d.UserID == userID {
			return &d, nil
		}
	}
	return nil, ports.ErrDriverNotFound
}

// Save reemplaza el repartidor solo si la versión almacenada es expectedVersion.
func (r *DriverRepository) Save(_ context.Context, d *domain.Driver, expectedVersion int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored, ok := r.drivers[d.ID]
	if !ok {
		return ports.ErrDriverNotFound
	}
	if stored.Version != expectedVersion {
		return ports.ErrVersionConflict
	}
	r.drivers[d.ID] = *d
	return nil
}

// FindAvailable devuelve hasta limit repartidores activos y AVAILABLE, ordenados por ID.
func (r *DriverRepository) FindAvailable(_ context.Context, limit int) ([]*domain.Driver, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0, len(r.drivers))
	for id := range r.drivers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]*domain.Driver, 0, limit)
	for _, id := range ids {
		if len(out) == limit {
			break
		}
		d := r.drivers[id]
		if d.Active && d.Status == domain.StatusAvailable {
			out = append(out, &d)
		}
	}
	return out, nil
}
