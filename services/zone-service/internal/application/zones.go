// Package application contiene los casos de uso del Zone Service.
package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/domain"
)

// maxCodeAttempts acota los reintentos cuando un código generado ya existe (por ejemplo, uno
// registrado a mano antes de que existiera la generación automática).
const maxCodeAttempts = 5

// ZoneService orquesta los casos de uso de zonas.
type ZoneService struct {
	zones ports.ZoneRepository
}

// NewZoneService construye el servicio con su repositorio.
func NewZoneService(zones ports.ZoneRepository) *ZoneService {
	return &ZoneService{zones: zones}
}

// CreateZone valida y registra una zona nueva sin huecos. Si el código va vacío, se genera.
func (s *ZoneService) CreateZone(ctx context.Context, code, name string, ring []domain.Coordinate) (domain.Zone, error) {
	return s.CreateZoneWithHoles(ctx, code, name, ring, nil)
}

// CreateZoneWithHoles valida y registra una zona nueva con huecos opcionales (dona). Si el código
// va vacío, el servidor lo genera a partir del nombre (NORTE-001) y lo asigna de forma atómica.
func (s *ZoneService) CreateZoneWithHoles(ctx context.Context, code, name string, ring []domain.Coordinate, holes [][]domain.Coordinate) (domain.Zone, error) {
	name, err := domain.ValidateName(name)
	if err != nil {
		return domain.Zone{}, err
	}
	if err := domain.ValidateGeometry(ring, holes); err != nil {
		return domain.Zone{}, err
	}

	if strings.TrimSpace(code) != "" {
		zone, err := domain.NewZoneWithHoles(code, name, ring, holes)
		if err != nil {
			return domain.Zone{}, err
		}
		return s.save(ctx, zone)
	}
	return s.createWithGeneratedCode(ctx, name, ring, holes)
}

func (s *ZoneService) createWithGeneratedCode(ctx context.Context, name string, ring []domain.Coordinate, holes [][]domain.Coordinate) (domain.Zone, error) {
	prefix := domain.CodePrefixFromName(name)
	for attempt := 0; attempt < maxCodeAttempts; attempt++ {
		number, err := s.zones.NextCode(ctx, prefix)
		if err != nil {
			return domain.Zone{}, fmt.Errorf("next zone code: %w", err)
		}
		zone, err := domain.NewZoneWithHoles(domain.FormatCode(prefix, number), name, ring, holes)
		if err != nil {
			return domain.Zone{}, err
		}
		saved, err := s.save(ctx, zone)
		if errors.Is(err, domain.ErrZoneCodeTaken) {
			continue
		}
		return saved, err
	}
	return domain.Zone{}, domain.ErrZoneCodeTaken
}

func (s *ZoneService) save(ctx context.Context, zone domain.Zone) (domain.Zone, error) {
	saved, err := s.zones.Save(ctx, zone)
	if err != nil {
		return domain.Zone{}, fmt.Errorf("create zone: %w", err)
	}
	return saved, nil
}

// UpdateZone redibuja una zona activa: reemplaza nombre y geometría con las mismas reglas que la
// creación. El código y el identificador no cambian. Una zona inactiva debe reactivarse primero.
func (s *ZoneService) UpdateZone(ctx context.Context, id, name string, ring []domain.Coordinate, holes [][]domain.Coordinate) (domain.Zone, error) {
	if _, err := uuid.Parse(id); err != nil {
		return domain.Zone{}, domain.ErrZoneNotFound
	}
	existing, err := s.zones.FindByID(ctx, id)
	if err != nil {
		return domain.Zone{}, err
	}
	if existing.Status != domain.StatusActive {
		return domain.Zone{}, domain.ErrZoneInactive
	}
	zone, err := domain.NewZoneWithHoles(existing.Code, name, ring, holes)
	if err != nil {
		return domain.Zone{}, err
	}
	zone.ID = id
	updated, err := s.zones.Update(ctx, zone)
	if err != nil {
		return domain.Zone{}, fmt.Errorf("update zone: %w", err)
	}
	return updated, nil
}

// SetZoneStatus activa o desactiva una zona. El motivo es obligatorio y queda en la bitácora.
func (s *ZoneService) SetZoneStatus(ctx context.Context, id, status, reason string) (domain.Zone, error) {
	if _, err := uuid.Parse(id); err != nil {
		return domain.Zone{}, domain.ErrZoneNotFound
	}
	if !domain.ValidStatus(status) {
		return domain.Zone{}, domain.ErrInvalidStatus
	}
	reason, err := domain.ValidateReason(reason)
	if err != nil {
		return domain.Zone{}, err
	}
	existing, err := s.zones.FindByID(ctx, id)
	if err != nil {
		return domain.Zone{}, err
	}
	if existing.Status == status {
		return domain.Zone{}, domain.ErrStatusUnchanged
	}
	return s.zones.SetStatus(ctx, id, status, reason)
}

// DeleteZone borra físicamente una zona inactiva. La bitácora conserva el registro del borrado.
func (s *ZoneService) DeleteZone(ctx context.Context, id, reason string) error {
	if _, err := uuid.Parse(id); err != nil {
		return domain.ErrZoneNotFound
	}
	reason, err := domain.ValidateReason(reason)
	if err != nil {
		return err
	}
	return s.zones.Delete(ctx, id, reason)
}

// GetZone devuelve una zona por su identificador, activa o no.
func (s *ZoneService) GetZone(ctx context.Context, id string) (domain.Zone, error) {
	if _, err := uuid.Parse(id); err != nil {
		return domain.Zone{}, domain.ErrZoneNotFound
	}
	return s.zones.FindByID(ctx, id)
}

// ListZones devuelve todas las zonas registradas, activas o no.
func (s *ZoneService) ListZones(ctx context.Context) ([]domain.Zone, error) {
	zones, err := s.zones.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list zones: %w", err)
	}
	return zones, nil
}

// ClassifyPoint devuelve la zona activa que contiene la coordenada.
func (s *ZoneService) ClassifyPoint(ctx context.Context, latitude, longitude float64) (domain.Zone, error) {
	point, err := domain.NewCoordinate(latitude, longitude)
	if err != nil {
		return domain.Zone{}, err
	}
	return s.zones.FindContaining(ctx, point)
}
