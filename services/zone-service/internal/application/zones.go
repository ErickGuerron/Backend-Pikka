// Package application contiene los casos de uso del Zone Service.
package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/domain"
)

// ZoneService orquesta los casos de uso de zonas.
type ZoneService struct {
	zones ports.ZoneRepository
}

// NewZoneService construye el servicio con su repositorio.
func NewZoneService(zones ports.ZoneRepository) *ZoneService {
	return &ZoneService{zones: zones}
}

// CreateZone valida y registra una zona nueva sin huecos.
func (s *ZoneService) CreateZone(ctx context.Context, code, name string, ring []domain.Coordinate) (domain.Zone, error) {
	return s.CreateZoneWithHoles(ctx, code, name, ring, nil)
}

// CreateZoneWithHoles valida y registra una zona nueva con huecos opcionales (dona).
func (s *ZoneService) CreateZoneWithHoles(ctx context.Context, code, name string, ring []domain.Coordinate, holes [][]domain.Coordinate) (domain.Zone, error) {
	zone, err := domain.NewZoneWithHoles(code, name, ring, holes)
	if err != nil {
		return domain.Zone{}, err
	}
	saved, err := s.zones.Save(ctx, zone)
	if err != nil {
		return domain.Zone{}, fmt.Errorf("create zone: %w", err)
	}
	return saved, nil
}

// GetZone devuelve una zona por su identificador.
func (s *ZoneService) GetZone(ctx context.Context, id string) (domain.Zone, error) {
	if _, err := uuid.Parse(id); err != nil {
		return domain.Zone{}, domain.ErrZoneNotFound
	}
	return s.zones.FindByID(ctx, id)
}

// ClassifyPoint devuelve la zona que contiene la coordenada.
func (s *ZoneService) ClassifyPoint(ctx context.Context, latitude, longitude float64) (domain.Zone, error) {
	point, err := domain.NewCoordinate(latitude, longitude)
	if err != nil {
		return domain.Zone{}, err
	}
	return s.zones.FindContaining(ctx, point)
}
