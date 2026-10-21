// Package grpc expone el contrato drivers.v1 (DriverService) sobre gRPC.
package grpc

import (
	"context"
	"errors"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/driver"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

// StopSource entrega la dirección y la ventana de una parada de una ruta. Su origen es el
// Routing Service, que aún no existe.
type StopSource interface {
	Stop(ctx context.Context, routeID, stopID string) (domain.Stop, error)
}

// DriverServer implementa driversv1.DriverServiceServer delegando en la capa de aplicación.
type DriverServer struct {
	driversv1.UnimplementedDriverServiceServer
	svc        *driver.Service
	deliveries *driver.Deliveries
	stops      StopSource
}

// NewDriverServer crea el adaptador gRPC.
func NewDriverServer(svc *driver.Service) *DriverServer {
	return &DriverServer{svc: svc}
}

// WithDeliveries activa RegisterDelivery. Si stops es nil, el RPC responde UNIMPLEMENTED: sin
// fuente de paradas no hay forma verificable de comprobar la llegada.
func (s *DriverServer) WithDeliveries(deliveries *driver.Deliveries, stops StopSource) *DriverServer {
	s.deliveries = deliveries
	s.stops = stops
	return s
}

// GetDriverByUserID devuelve el resumen del repartidor asociado a un user_id.
func (s *DriverServer) GetDriverByUserID(ctx context.Context, req *driversv1.GetDriverByUserIDRequest) (*driversv1.GetDriverByUserIDResponse, error) {
	userID := strings.TrimSpace(req.GetUserId())
	if userID == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	d, err := s.svc.FindByUserID(ctx, userID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &driversv1.GetDriverByUserIDResponse{Driver: &driversv1.DriverSummary{Id: d.ID, UserId: d.UserID, RouteId: d.RouteID}}, nil
}

// GetDriverByRouteID devuelve solo los identificadores del repartidor reservado para la ruta.
func (s *DriverServer) GetDriverByRouteID(ctx context.Context, req *driversv1.GetDriverByRouteIDRequest) (*driversv1.GetDriverByRouteIDResponse, error) {
	routeID := strings.TrimSpace(req.GetRouteId())
	if routeID == "" {
		return nil, status.Error(codes.InvalidArgument, "route_id is required")
	}
	d, err := s.svc.FindByRouteID(ctx, routeID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &driversv1.GetDriverByRouteIDResponse{DriverId: d.ID, UserId: d.UserID}, nil
}

// RegisterDelivery registra el resultado de una parada. La parada la aporta la fuente de
// paradas, nunca el llamador.
func (s *DriverServer) RegisterDelivery(ctx context.Context, req *driversv1.RegisterDeliveryRequest) (*driversv1.RegisterDeliveryResponse, error) {
	if s.deliveries == nil || s.stops == nil {
		return nil, status.Error(codes.Unimplemented, "stop source is not available yet")
	}
	recordedAt, err := time.Parse(time.RFC3339, req.GetRecordedAt())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "recorded_at must be RFC 3339")
	}
	routeID := strings.TrimSpace(req.GetRouteId())
	stopID := strings.TrimSpace(req.GetStopId())
	stop, err := s.stops.Stop(ctx, routeID, stopID)
	if err != nil {
		return nil, toStatus(err)
	}
	in := driver.DeliveryInput{
		StopID:   stopID,
		Outcome:  domain.Outcome(req.GetOutcome()),
		Reason:   domain.NotDeliveredReason(req.GetReason()),
		At:       recordedAt,
		Location: domain.GeoPoint{Latitude: req.GetLatitude(), Longitude: req.GetLongitude()},
		PhotoURL: req.GetPhotoUrl(),
	}
	reg, err := s.deliveries.RegisterDelivery(ctx, strings.TrimSpace(req.GetDriverId()), routeID, stop, in)
	if err != nil {
		return nil, toStatus(err)
	}
	return &driversv1.RegisterDeliveryResponse{
		StopId:         reg.Record.StopID,
		Outcome:        string(reg.Record.Outcome),
		Reason:         string(reg.Record.Reason),
		WithinRadius:   reg.Arrival.WithinRadius,
		WithinWindow:   reg.Arrival.WithinWindow,
		DistanceMeters: reg.Arrival.DistanceM,
	}, nil
}

// CreateDriver registra el perfil de un repartidor nuevo.
func (s *DriverServer) CreateDriver(ctx context.Context, req *driversv1.CreateDriverRequest) (*driversv1.CreateDriverResponse, error) {
	birth, err := time.Parse(time.DateOnly, strings.TrimSpace(req.GetBirthDate()))
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "birth_date must be YYYY-MM-DD")
	}
	vehicle, err := fromProtoVehicle(req.GetVehicleType())
	if err != nil {
		return nil, toStatus(err)
	}
	d, err := s.svc.Create(ctx, domain.NewDriverInput{
		UserID:      req.GetUserId(),
		FirstName:   req.GetFirstName(),
		LastName:    req.GetLastName(),
		BirthDate:   birth,
		Phone:       req.GetPhone(),
		VehicleType: string(vehicle),
		Identifier:  req.GetIdentifier(),
		Plate:       req.GetPlate(),
		City:        req.GetCity(),
	})
	if err != nil {
		return nil, toStatus(err)
	}
	return &driversv1.CreateDriverResponse{Driver: toProtoProfile(d)}, nil
}

// ListDrivers devuelve la flota para el personal, sin fecha de nacimiento ni identificador.
func (s *DriverServer) ListDrivers(ctx context.Context, _ *driversv1.ListDriversRequest) (*driversv1.ListDriversResponse, error) {
	list, err := s.svc.List(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	out := make([]*driversv1.DriverListItem, 0, len(list))
	for _, d := range list {
		out = append(out, &driversv1.DriverListItem{
			Id: d.ID, UserId: d.UserID, FirstName: d.FirstName, LastName: d.LastName, Phone: d.Phone, City: d.City,
			VehicleType: toProtoVehicle(d.VehicleType), Plate: d.Plate, Active: d.Active, Status: toProtoStatus(d.Status),
		})
	}
	return &driversv1.ListDriversResponse{Drivers: out}, nil
}

// FindAvailableDrivers devuelve solo identificadores de repartidores activos y disponibles.
func (s *DriverServer) FindAvailableDrivers(ctx context.Context, req *driversv1.FindAvailableDriversRequest) (*driversv1.FindAvailableDriversResponse, error) {
	list, err := s.svc.FindAvailableDrivers(ctx, int(req.GetLimit()))
	if err != nil {
		return nil, toStatus(err)
	}
	out := make([]*driversv1.DriverSummary, 0, len(list))
	for _, d := range list {
		out = append(out, &driversv1.DriverSummary{Id: d.ID, UserId: d.UserID})
	}
	return &driversv1.FindAvailableDriversResponse{Drivers: out}, nil
}

// ReserveDriver reserva al repartidor para la ruta. Repetir la misma ruta es idempotente.
func (s *DriverServer) ReserveDriver(ctx context.Context, req *driversv1.ReserveDriverRequest) (*driversv1.ReserveDriverResponse, error) {
	driverID := strings.TrimSpace(req.GetDriverId())
	routeID := strings.TrimSpace(req.GetRouteId())
	if driverID == "" {
		return nil, status.Error(codes.InvalidArgument, "driver_id is required")
	}
	if routeID == "" {
		return nil, status.Error(codes.InvalidArgument, "route_id is required")
	}
	d, err := s.svc.ReserveDriver(ctx, driverID, routeID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &driversv1.ReserveDriverResponse{Driver: toProto(d)}, nil
}

// StartRoute marca al repartidor como ON_ROUTE en su ruta reservada. Lo llama el gateway con el primer
// PICKED_UP de la ruta. Sin gate de rol: el gateway ya verificó que la ruta es del repartidor.
func (s *DriverServer) StartRoute(ctx context.Context, req *driversv1.StartRouteRequest) (*driversv1.StartRouteResponse, error) {
	driverID := strings.TrimSpace(req.GetDriverId())
	routeID := strings.TrimSpace(req.GetRouteId())
	if driverID == "" {
		return nil, status.Error(codes.InvalidArgument, "driver_id is required")
	}
	if routeID == "" {
		return nil, status.Error(codes.InvalidArgument, "route_id is required")
	}
	d, err := s.svc.StartRoute(ctx, driverID, routeID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &driversv1.StartRouteResponse{Driver: toProto(d)}, nil
}

// ReleaseDriver frees the reservation of driver_id for route_id. Like ReserveDriver, it has no
// role gate: it is called by the Routing Service, which is internal.
func (s *DriverServer) ReleaseDriver(ctx context.Context, req *driversv1.ReleaseDriverRequest) (*driversv1.ReleaseDriverResponse, error) {
	driverID := strings.TrimSpace(req.GetDriverId())
	routeID := strings.TrimSpace(req.GetRouteId())
	if driverID == "" {
		return nil, status.Error(codes.InvalidArgument, "driver_id is required")
	}
	if routeID == "" {
		return nil, status.Error(codes.InvalidArgument, "route_id is required")
	}
	d, err := s.svc.ReleaseDriver(ctx, driverID, routeID)
	if err != nil {
		return nil, toStatus(err)
	}
	return &driversv1.ReleaseDriverResponse{Driver: toProto(d)}, nil
}

// toStatus traduce errores de dominio y de aplicación a los códigos del contrato.
// FAILED_PRECONDITION distingue sus tres causas con mensajes distintos.
func toStatus(err error) error {
	switch {
	case errors.Is(err, driver.ErrInvalidLimit):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, ports.ErrDriverNotFound):
		return status.Error(codes.NotFound, "driver not found")
	case errors.Is(err, domain.ErrDriverInactive):
		return status.Error(codes.FailedPrecondition, "driver is inactive")
	case errors.Is(err, domain.ErrDriverOffline):
		return status.Error(codes.FailedPrecondition, "driver is offline")
	case errors.Is(err, domain.ErrDriverAlreadyReserved):
		return status.Error(codes.FailedPrecondition, "driver is already reserved for another route")
	case errors.Is(err, domain.ErrOutsideRadius):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, domain.ErrRouteMismatch), errors.Is(err, domain.ErrInvalidTransition):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, domain.ErrInvalidRouteID), errors.Is(err, domain.ErrInvalidDriverID):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, ports.ErrStopNotFound):
		return status.Error(codes.NotFound, "stop not found")
	case errors.Is(err, ports.ErrDeliveryAlreadyRecorded):
		return status.Error(codes.AlreadyExists, "delivery already recorded for this stop")
	case errors.Is(err, domain.ErrInvalidOutcome), errors.Is(err, domain.ErrReasonRequired),
		errors.Is(err, domain.ErrReasonNotAllowed), errors.Is(err, domain.ErrInvalidStopID),
		errors.Is(err, domain.ErrInvalidCoordinates):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, ports.ErrDriverExists):
		return status.Error(codes.AlreadyExists, "user, identifier or plate already belongs to a driver")
	case errors.Is(err, domain.ErrInvalidUserID), errors.Is(err, domain.ErrInvalidFirstName),
		errors.Is(err, domain.ErrInvalidLastName), errors.Is(err, domain.ErrInvalidBirthDate),
		errors.Is(err, domain.ErrInvalidPhone), errors.Is(err, domain.ErrInvalidVehicleType),
		errors.Is(err, domain.ErrInvalidIdentifier), errors.Is(err, domain.ErrInvalidPlate),
		errors.Is(err, domain.ErrInvalidCity):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, ports.ErrVersionConflict):
		// Tras los reintentos del servicio, otro cambio sigue ganando: el cliente debe reintentar.
		return status.Error(codes.Aborted, "driver changed concurrently, retry the request")
	default:
		return status.Error(codes.Internal, "internal error")
	}
}

func toProto(d *domain.Driver) *driversv1.Driver {
	return &driversv1.Driver{
		Id:        d.ID,
		UserId:    d.UserID,
		FullName:  d.FullName,
		Phone:     d.Phone,
		Active:    d.Active,
		Status:    toProtoStatus(d.Status),
		CreatedAt: d.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func toProtoProfile(d *domain.Driver) *driversv1.DriverProfile {
	return &driversv1.DriverProfile{
		Id:          d.ID,
		UserId:      d.UserID,
		FirstName:   d.FirstName,
		LastName:    d.LastName,
		BirthDate:   d.BirthDate.Format(time.DateOnly),
		Phone:       d.Phone,
		VehicleType: toProtoVehicle(d.VehicleType),
		Identifier:  d.Identifier,
		Plate:       d.Plate,
		City:        d.City,
		Active:      d.Active,
		Status:      toProtoStatus(d.Status),
		CreatedAt:   d.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func toProtoVehicle(v domain.VehicleType) driversv1.VehicleType {
	switch v {
	case domain.VehicleMotorbike:
		return driversv1.VehicleType_VEHICLE_TYPE_MOTORBIKE
	case domain.VehicleCar:
		return driversv1.VehicleType_VEHICLE_TYPE_CAR
	case domain.VehicleSUV:
		return driversv1.VehicleType_VEHICLE_TYPE_SUV
	default:
		return driversv1.VehicleType_VEHICLE_TYPE_UNSPECIFIED
	}
}

func fromProtoVehicle(v driversv1.VehicleType) (domain.VehicleType, error) {
	switch v {
	case driversv1.VehicleType_VEHICLE_TYPE_MOTORBIKE:
		return domain.VehicleMotorbike, nil
	case driversv1.VehicleType_VEHICLE_TYPE_CAR:
		return domain.VehicleCar, nil
	case driversv1.VehicleType_VEHICLE_TYPE_SUV:
		return domain.VehicleSUV, nil
	default:
		return "", domain.ErrInvalidVehicleType
	}
}

func toProtoStatus(s domain.Status) driversv1.DriverStatus {
	switch s {
	case domain.StatusAvailable:
		return driversv1.DriverStatus_DRIVER_STATUS_AVAILABLE
	case domain.StatusAssigned:
		return driversv1.DriverStatus_DRIVER_STATUS_ASSIGNED
	case domain.StatusOnRoute:
		return driversv1.DriverStatus_DRIVER_STATUS_ON_ROUTE
	case domain.StatusOffline:
		return driversv1.DriverStatus_DRIVER_STATUS_OFFLINE
	default:
		return driversv1.DriverStatus_DRIVER_STATUS_UNSPECIFIED
	}
}
