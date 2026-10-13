// Package driver contiene los casos de uso de repartidores.
package driver

import (
	"context"
	"errors"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

const (
	defaultLimit = 50
	maxLimit     = 100
	// maxAttempts acota los reintentos ante conflictos de versión. Cada reintento vuelve a
	// leer el repartidor, así que una reserva concurrente siempre termina en la regla del
	// dominio (por ejemplo ErrDriverAlreadyReserved), no en un error técnico.
	maxAttempts = 5
)

// ErrInvalidLimit indica un límite fuera de rango para FindAvailableDrivers.
var ErrInvalidLimit = errors.New("limit must be between 1 and 100")

// Service coordina los cambios de estado de los repartidores.
type Service struct {
	repo ports.DriverRepository
	now  func() time.Time
}

// NewService crea el servicio. now se inyecta para que las pruebas controlen el reloj.
func NewService(repo ports.DriverRepository, now func() time.Time) *Service {
	return &Service{repo: repo, now: now}
}

// SetActive activa o desactiva al repartidor. Es idempotente.
func (s *Service) SetActive(ctx context.Context, id string, active bool) (*domain.Driver, error) {
	return s.mutate(ctx, id, func(d *domain.Driver, now time.Time) error {
		if active {
			d.Activate(now)
		} else {
			d.Deactivate(now)
		}
		return nil
	})
}

// GoOnline conecta al repartidor fuera de línea a la operación.
func (s *Service) GoOnline(ctx context.Context, id string) (*domain.Driver, error) {
	return s.mutate(ctx, id, func(d *domain.Driver, now time.Time) error {
		return d.GoOnline(now)
	})
}

// GoOffline retira al repartidor de la operación.
func (s *Service) GoOffline(ctx context.Context, id string) (*domain.Driver, error) {
	return s.mutate(ctx, id, func(d *domain.Driver, now time.Time) error {
		return d.GoOffline(now)
	})
}

// ReserveDriver reserva al repartidor para la ruta routeID. Repetir la misma ruta es un
// reintento idempotente.
func (s *Service) ReserveDriver(ctx context.Context, id, routeID string) (*domain.Driver, error) {
	return s.mutate(ctx, id, func(d *domain.Driver, now time.Time) error {
		return d.Reserve(routeID, now)
	})
}

// FindAvailableDrivers devuelve repartidores activos y disponibles. limit 0 usa el valor
// por defecto (50); fuera de 1..100 se rechaza.
func (s *Service) FindAvailableDrivers(ctx context.Context, limit int) ([]*domain.Driver, error) {
	switch {
	case limit == 0:
		limit = defaultLimit
	case limit < 1 || limit > maxLimit:
		return nil, ErrInvalidLimit
	}
	return s.repo.FindAvailable(ctx, limit)
}

// mutate lee el repartidor, aplica el cambio y lo guarda con control optimista. Ante un
// conflicto de versión vuelve a leer y reaplica el cambio, hasta maxAttempts veces.
func (s *Service) mutate(ctx context.Context, id string, change func(*domain.Driver, time.Time) error) (*domain.Driver, error) {
	for attempt := 0; attempt < maxAttempts; attempt++ {
		d, err := s.repo.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		expected := d.Version
		if err := change(d, s.now()); err != nil {
			return nil, err
		}
		err = s.repo.Save(ctx, d, expected)
		if errors.Is(err, ports.ErrVersionConflict) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return d, nil
	}
	return nil, ports.ErrVersionConflict
}
