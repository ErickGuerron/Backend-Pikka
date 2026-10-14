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

// DriverServer implementa driversv1.DriverServiceServer delegando en la capa de aplicación.
type DriverServer struct {
	driversv1.UnimplementedDriverServiceServer
	svc *driver.Service
}

// NewDriverServer crea el adaptador gRPC.
func NewDriverServer(svc *driver.Service) *DriverServer {
	return &DriverServer{svc: svc}
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
	case errors.Is(err, domain.ErrRouteMismatch), errors.Is(err, domain.ErrInvalidTransition):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, domain.ErrInvalidRouteID), errors.Is(err, domain.ErrInvalidDriverID):
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
