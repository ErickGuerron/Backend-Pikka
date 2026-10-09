// Package grpc expone los casos de uso del Zone Service mediante gRPC.
package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	zonesv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/zones/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/application"
	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/domain"
)

// Server implementa zonesv1.ZoneServiceServer. Solo traduce; no contiene reglas de negocio.
type Server struct {
	zonesv1.UnimplementedZoneServiceServer
	zones *application.ZoneService
}

// NewServer crea el servidor gRPC sobre los casos de uso de zonas.
func NewServer(zones *application.ZoneService) *Server {
	return &Server{zones: zones}
}

// CreateZone registra una zona nueva.
func (s *Server) CreateZone(ctx context.Context, req *zonesv1.CreateZoneRequest) (*zonesv1.CreateZoneResponse, error) {
	ring := fromProtoRing(req.GetRing())
	zone, err := s.zones.CreateZone(ctx, req.GetCode(), req.GetName(), ring)
	if err != nil {
		return nil, toStatus(err)
	}
	return &zonesv1.CreateZoneResponse{Zone: toProtoZone(zone)}, nil
}

// GetZone devuelve una zona por su identificador.
func (s *Server) GetZone(ctx context.Context, req *zonesv1.GetZoneRequest) (*zonesv1.GetZoneResponse, error) {
	zone, err := s.zones.GetZone(ctx, req.GetId())
	if err != nil {
		return nil, toStatus(err)
	}
	return &zonesv1.GetZoneResponse{Zone: toProtoZone(zone)}, nil
}

// ClassifyPoint devuelve la zona que contiene la coordenada.
func (s *Server) ClassifyPoint(ctx context.Context, req *zonesv1.ClassifyPointRequest) (*zonesv1.ClassifyPointResponse, error) {
	point := req.GetPoint()
	zone, err := s.zones.ClassifyPoint(ctx, point.GetLatitude(), point.GetLongitude())
	if err != nil {
		return nil, toStatus(err)
	}
	return &zonesv1.ClassifyPointResponse{Zone: toProtoZone(zone)}, nil
}

func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidCode),
		errors.Is(err, domain.ErrInvalidName),
		errors.Is(err, domain.ErrInvalidCoordinate),
		errors.Is(err, domain.ErrInvalidRing),
		errors.Is(err, domain.ErrRingNotClosed):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrZoneNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrZoneCodeTaken):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "deadline exceeded")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "canceled")
	default:
		// El detalle se pierde aquí a propósito: no se filtran errores de base de datos al cliente.
		return status.Error(codes.Internal, "internal error")
	}
}

func fromProtoRing(points []*zonesv1.Coordinate) []domain.Coordinate {
	ring := make([]domain.Coordinate, 0, len(points))
	for _, p := range points {
		// Las coordenadas inválidas las rechaza el dominio con InvalidArgument.
		ring = append(ring, domain.Coordinate{Latitude: p.GetLatitude(), Longitude: p.GetLongitude()})
	}
	return ring
}

func toProtoZone(z domain.Zone) *zonesv1.Zone {
	ring := make([]*zonesv1.Coordinate, 0, len(z.Ring))
	for _, p := range z.Ring {
		ring = append(ring, &zonesv1.Coordinate{Latitude: p.Latitude, Longitude: p.Longitude})
	}
	return &zonesv1.Zone{
		Id:        z.ID,
		Code:      z.Code,
		Name:      z.Name,
		Ring:      ring,
		CreatedAt: z.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}
