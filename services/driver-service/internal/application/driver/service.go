// Package driver contiene los casos de uso de repartidores.
package driver

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
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

// ErrInvalidLimit indica un límite negativo para FindAvailableDrivers.
var ErrInvalidLimit = errors.New("limit must not be negative")

// Service coordina los cambios de estado de los repartidores.
type Service struct {
	repo ports.DriverRepository
	now  func() time.Time
}

// NewService crea el servicio. now se inyecta para que las pruebas controlen el reloj.
func NewService(repo ports.DriverRepository, now func() time.Time) *Service {
	return &Service{repo: repo, now: now}
}

// Create registra un repartidor nuevo con su perfil. Los datos duplicados (usuario,
// identificador o placa) los rechaza el repositorio con ports.ErrDriverExists.
func (s *Service) Create(ctx context.Context, in domain.NewDriverInput) (*domain.Driver, error) {
	d, err := domain.NewDriver(newID(), in, s.now())
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

// newID genera un UUID v4.
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // sin entropía del sistema no se puede continuar
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
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

// ReleaseDriver frees the reservation of driver id for routeID. It is idempotent; see
// domain.Driver.ReleaseRoute.
func (s *Service) ReleaseDriver(ctx context.Context, id, routeID string) (*domain.Driver, error) {
	return s.mutate(ctx, id, func(d *domain.Driver, now time.Time) error {
		return d.ReleaseRoute(routeID, now)
	})
}

// FindByUserID devuelve el repartidor asociado a un usuario de Auth Service.
func (s *Service) FindByUserID(ctx context.Context, userID string) (*domain.Driver, error) {
	return s.repo.FindByUserID(ctx, userID)
}

// FindByRouteID devuelve el repartidor reservado para la ruta routeID. Un routeID en blanco
// es ErrInvalidRouteID; si ningún repartidor tiene esa ruta, ErrDriverNotFound.
func (s *Service) FindByRouteID(ctx context.Context, routeID string) (*domain.Driver, error) {
	routeID = strings.TrimSpace(routeID)
	if routeID == "" {
		return nil, domain.ErrInvalidRouteID
	}
	return s.repo.FindByRouteID(ctx, routeID)
}

// ListLimit es el máximo de repartidores que devuelve List.
const ListLimit = 500

// List devuelve la flota completa (activos e inactivos) para el personal, hasta ListLimit.
func (s *Service) List(ctx context.Context) ([]*domain.Driver, error) {
	return s.repo.List(ctx, ListLimit)
}

// FindAvailableDrivers devuelve repartidores activos y disponibles. limit 0 usa el valor
// por defecto (50), un valor mayor que 100 se recorta a 100 y un negativo se rechaza. Así
// lo define el contrato drivers.v1.
func (s *Service) FindAvailableDrivers(ctx context.Context, limit int) ([]*domain.Driver, error) {
	switch {
	case limit < 0:
		return nil, ErrInvalidLimit
	case limit == 0:
		limit = defaultLimit
	case limit > maxLimit:
		limit = maxLimit
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
