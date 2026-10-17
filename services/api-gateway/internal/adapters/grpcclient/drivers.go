package grpcclient

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
)

// Drivers envuelve al cliente del Driver Service para los endpoints de repartidor.
// Impone deadline y propaga la identidad del Gateway, igual que Orders.
type Drivers struct {
	client  driversv1.DriverServiceClient
	timeout time.Duration
}

// NewDrivers crea el envoltorio sobre una conexión gRPC existente.
func NewDrivers(conn grpc.ClientConnInterface, timeout time.Duration) *Drivers {
	return &Drivers{client: driversv1.NewDriverServiceClient(conn), timeout: timeout}
}

// NewDriversWithClient permite inyectar un cliente falso en pruebas.
func NewDriversWithClient(client driversv1.DriverServiceClient, timeout time.Duration) *Drivers {
	return &Drivers{client: client, timeout: timeout}
}

func (d *Drivers) outgoing(ctx context.Context, m CallMeta) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	pairs := []string{MetadataRequestID, m.RequestID}
	if m.UserID != "" {
		pairs = append(pairs, MetadataUserID, m.UserID, MetadataUserRole, m.Role)
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...), cancel
}

// GetDriverByUserID devuelve el repartidor asociado a un usuario autenticado.
func (d *Drivers) GetDriverByUserID(ctx context.Context, m CallMeta, req *driversv1.GetDriverByUserIDRequest) (*driversv1.GetDriverByUserIDResponse, error) {
	ctx, cancel := d.outgoing(ctx, m)
	defer cancel()
	return d.client.GetDriverByUserID(ctx, req)
}

// RegisterDelivery registra el resultado de una parada de la ruta del repartidor.
func (d *Drivers) RegisterDelivery(ctx context.Context, m CallMeta, req *driversv1.RegisterDeliveryRequest) (*driversv1.RegisterDeliveryResponse, error) {
	ctx, cancel := d.outgoing(ctx, m)
	defer cancel()
	return d.client.RegisterDelivery(ctx, req)
}

// CreateDriver registra el perfil de un repartidor cuyo usuario DRIVER ya existe.
func (d *Drivers) CreateDriver(ctx context.Context, m CallMeta, req *driversv1.CreateDriverRequest) (*driversv1.CreateDriverResponse, error) {
	ctx, cancel := d.outgoing(ctx, m)
	defer cancel()
	return d.client.CreateDriver(ctx, req)
}

// ListDrivers devuelve la flota para el personal.
func (d *Drivers) ListDrivers(ctx context.Context, m CallMeta, req *driversv1.ListDriversRequest) (*driversv1.ListDriversResponse, error) {
	ctx, cancel := d.outgoing(ctx, m)
	defer cancel()
	return d.client.ListDrivers(ctx, req)
}
