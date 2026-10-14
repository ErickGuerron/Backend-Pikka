// Package grpc expone los casos de uso del Zone Service mediante gRPC.
package grpc

import (
	"context"
	"errors"
	"time"

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
	if err := requireRole(ctx, roleAdmin); err != nil {
		return nil, err
	}
	ctx, err := withActor(ctx)
	if err != nil {
		return nil, err
	}
	ring := fromProtoRing(req.GetRing())
	holes := fromProtoHoles(req.GetHoles())
	zone, err := s.zones.CreateZoneWithHoles(ctx, req.GetCode(), req.GetName(), ring, holes, fromProtoSettings(req.GetSettings()))
	if err != nil {
		return nil, toStatus(err)
	}
	return &zonesv1.CreateZoneResponse{Zone: toProtoZone(zone)}, nil
}

// ListZones devuelve todas las zonas registradas.
func (s *Server) ListZones(ctx context.Context, _ *zonesv1.ListZonesRequest) (*zonesv1.ListZonesResponse, error) {
	if err := requireRole(ctx, roleAdmin, roleOperator); err != nil {
		return nil, err
	}
	zones, err := s.zones.ListZones(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	out := make([]*zonesv1.Zone, 0, len(zones))
	for _, z := range zones {
		out = append(out, toProtoZone(z))
	}
	return &zonesv1.ListZonesResponse{Zones: out}, nil
}

// UpdateZone reemplaza los datos y la geometría de una zona existente.
func (s *Server) UpdateZone(ctx context.Context, req *zonesv1.UpdateZoneRequest) (*zonesv1.UpdateZoneResponse, error) {
	if err := requireRole(ctx, roleAdmin); err != nil {
		return nil, err
	}
	ctx, err := withActor(ctx)
	if err != nil {
		return nil, err
	}
	zone, err := s.zones.UpdateZone(ctx, req.GetId(), req.GetName(), fromProtoRing(req.GetRing()), fromProtoHoles(req.GetHoles()), fromProtoSettings(req.GetSettings()), int(req.GetExpectedVersion()))
	if err != nil {
		return nil, toStatus(err)
	}
	return &zonesv1.UpdateZoneResponse{Zone: toProtoZone(zone)}, nil
}

// SetZoneStatus activa o desactiva una zona. El motivo es obligatorio y queda en la bitácora.
func (s *Server) SetZoneStatus(ctx context.Context, req *zonesv1.SetZoneStatusRequest) (*zonesv1.SetZoneStatusResponse, error) {
	if err := requireRole(ctx, roleAdmin); err != nil {
		return nil, err
	}
	ctx, err := withActor(ctx)
	if err != nil {
		return nil, err
	}
	zone, err := s.zones.SetZoneStatus(ctx, req.GetId(), req.GetStatus(), req.GetReason())
	if err != nil {
		return nil, toStatus(err)
	}
	return &zonesv1.SetZoneStatusResponse{Zone: toProtoZone(zone)}, nil
}

// DeleteZone borra físicamente una zona inactiva.
func (s *Server) DeleteZone(ctx context.Context, req *zonesv1.DeleteZoneRequest) (*zonesv1.DeleteZoneResponse, error) {
	if err := requireRole(ctx, roleAdmin); err != nil {
		return nil, err
	}
	ctx, err := withActor(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.zones.DeleteZone(ctx, req.GetId(), req.GetReason()); err != nil {
		return nil, toStatus(err)
	}
	return &zonesv1.DeleteZoneResponse{}, nil
}

// GetZone devuelve una zona por su identificador.
func (s *Server) GetZone(ctx context.Context, req *zonesv1.GetZoneRequest) (*zonesv1.GetZoneResponse, error) {
	if err := requireRole(ctx, roleAdmin, roleOperator); err != nil {
		return nil, err
	}
	zone, err := s.zones.GetZone(ctx, req.GetId())
	if err != nil {
		return nil, toStatus(err)
	}
	return &zonesv1.GetZoneResponse{Zone: toProtoZone(zone)}, nil
}

// ClassifyPoint devuelve la zona que contiene la coordenada. Lo usan los
// operadores y, para validar pedidos, el Order Service con identidad de servicio.
func (s *Server) ClassifyPoint(ctx context.Context, req *zonesv1.ClassifyPointRequest) (*zonesv1.ClassifyPointResponse, error) {
	if err := requireRole(ctx, roleAdmin, roleOperator, roleService); err != nil {
		return nil, err
	}
	point := req.GetPoint()
	if point == nil {
		// Sin este chequeo, un punto ausente se leería como (0,0) y clasificaría mal.
		return nil, status.Error(codes.InvalidArgument, "point is required")
	}
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
		errors.Is(err, domain.ErrRingNotClosed),
		errors.Is(err, domain.ErrNotConvex),
		errors.Is(err, domain.ErrHoleOutside),
		errors.Is(err, domain.ErrHolesOverlap),
		errors.Is(err, domain.ErrReasonRequired),
		errors.Is(err, domain.ErrInvalidStatus),
		errors.Is(err, domain.ErrInvalidSettings):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrZoneNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrZoneCodeTaken):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, domain.ErrVersionConflict):
		// Otra edición ganó la carrera: el cliente debe recargar la zona y reintentar.
		return status.Error(codes.Aborted, err.Error())
	case errors.Is(err, domain.ErrStatusUnchanged),
		errors.Is(err, domain.ErrZoneNotInactive),
		errors.Is(err, domain.ErrZoneInactive):
		// El recurso existe pero su estado no permite la operación.
		return status.Error(codes.FailedPrecondition, err.Error())
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

// fromProtoHoles convierte los huecos del request. Un hueco sin puntos se rechaza en el dominio.
func fromProtoHoles(holes []*zonesv1.Ring) [][]domain.Coordinate {
	out := make([][]domain.Coordinate, 0, len(holes))
	for _, h := range holes {
		out = append(out, fromProtoRing(h.GetPoints()))
	}
	return out
}

func toProtoRing(points []domain.Coordinate) *zonesv1.Ring {
	ring := make([]*zonesv1.Coordinate, 0, len(points))
	for _, p := range points {
		ring = append(ring, &zonesv1.Coordinate{Latitude: p.Latitude, Longitude: p.Longitude})
	}
	return &zonesv1.Ring{Points: ring}
}

func toProtoZone(z domain.Zone) *zonesv1.Zone {
	ring := make([]*zonesv1.Coordinate, 0, len(z.Ring))
	for _, p := range z.Ring {
		ring = append(ring, &zonesv1.Coordinate{Latitude: p.Latitude, Longitude: p.Longitude})
	}
	holes := make([]*zonesv1.Ring, 0, len(z.Holes))
	for _, h := range z.Holes {
		holes = append(holes, toProtoRing(h))
	}
	return &zonesv1.Zone{
		Id:           z.ID,
		Code:         z.Code,
		Name:         z.Name,
		Ring:         ring,
		Status:       z.Status,
		StatusReason: z.StatusReason,
		CreatedAt:    formatTime(z.CreatedAt),
		Holes:        holes,
		Settings:     toProtoSettings(z.Settings),
		Version:      int32(z.Version),
		UpdatedAt:    formatTime(z.UpdatedAt),
		CreatedBy:    z.CreatedBy,
		UpdatedBy:    z.UpdatedBy,
	}
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

// fromProtoSettings convierte la configuración del request. Un request sin configuración llega
// como nil y el dominio la rechaza con InvalidArgument por los campos obligatorios.
func fromProtoSettings(s *zonesv1.ZoneSettings) domain.Settings {
	if s == nil {
		return domain.Settings{}
	}
	return domain.Settings{
		Description:        s.GetDescription(),
		DepotName:          s.GetDepotName(),
		DepotLatitude:      s.GetDepotLatitude(),
		DepotLongitude:     s.GetDepotLongitude(),
		MaxOrdersPerRoute:  int(s.GetMaxOrdersPerRoute()),
		RouteCapacityKg:    s.GetDefaultRouteCapacityKg(),
		Timezone:           s.GetTimezone(),
		ServiceDays:        s.GetServiceDays(),
		DeliveryWindowFrom: s.GetDeliveryWindowStart(),
		DeliveryWindowTo:   s.GetDeliveryWindowEnd(),
		CutoffTime:         s.GetCutoffTime(),
		DispatchTime:       s.GetDispatchTime(),
		Priority:           s.GetPriority(),
		DailyOrderLimit:    int(s.GetDailyOrderLimit()),
	}
}

func toProtoSettings(s domain.Settings) *zonesv1.ZoneSettings {
	return &zonesv1.ZoneSettings{
		Description:            s.Description,
		DepotName:              s.DepotName,
		DepotLatitude:          s.DepotLatitude,
		DepotLongitude:         s.DepotLongitude,
		MaxOrdersPerRoute:      int32(s.MaxOrdersPerRoute),
		DefaultRouteCapacityKg: s.RouteCapacityKg,
		Timezone:               s.Timezone,
		ServiceDays:            s.ServiceDays,
		DeliveryWindowStart:    s.DeliveryWindowFrom,
		DeliveryWindowEnd:      s.DeliveryWindowTo,
		CutoffTime:             s.CutoffTime,
		DispatchTime:           s.DispatchTime,
		Priority:               s.Priority,
		DailyOrderLimit:        int32(s.DailyOrderLimit),
	}
}
