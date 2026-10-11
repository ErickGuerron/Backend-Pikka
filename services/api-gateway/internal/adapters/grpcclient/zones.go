package grpcclient

import (
	"context"
	"time"

	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"

	zonesv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/zones/v1"
)

// Zones envuelve al cliente del Zone Service imponiendo deadline y propagando metadata.
type Zones struct {
	client  zonesv1.ZoneServiceClient
	health  healthpb.HealthClient
	timeout time.Duration
}

// NewZones crea el envoltorio sobre una conexión gRPC.
func NewZones(conn grpc.ClientConnInterface, timeout time.Duration) *Zones {
	return &Zones{client: zonesv1.NewZoneServiceClient(conn), health: healthpb.NewHealthClient(conn), timeout: timeout}
}

// NewZonesWithClient permite inyectar un cliente falso en pruebas.
func NewZonesWithClient(client zonesv1.ZoneServiceClient, timeout time.Duration) *Zones {
	return &Zones{client: client, timeout: timeout}
}

func (z *Zones) outgoing(ctx context.Context, m CallMeta) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, z.timeout)
	pairs := []string{MetadataRequestID, m.RequestID}
	if m.UserID != "" {
		pairs = append(pairs, MetadataUserID, m.UserID, MetadataUserRole, m.Role)
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...), cancel
}

// CreateZone solicita el registro de una zona.
func (z *Zones) CreateZone(ctx context.Context, m CallMeta, req *zonesv1.CreateZoneRequest) (*zonesv1.CreateZoneResponse, error) {
	ctx, cancel := z.outgoing(ctx, m)
	defer cancel()
	return z.client.CreateZone(ctx, req)
}

// GetZone devuelve una zona por id.
func (z *Zones) GetZone(ctx context.Context, m CallMeta, req *zonesv1.GetZoneRequest) (*zonesv1.GetZoneResponse, error) {
	ctx, cancel := z.outgoing(ctx, m)
	defer cancel()
	return z.client.GetZone(ctx, req)
}

// ListZones devuelve todas las zonas registradas.
func (z *Zones) ListZones(ctx context.Context, m CallMeta, req *zonesv1.ListZonesRequest) (*zonesv1.ListZonesResponse, error) {
	ctx, cancel := z.outgoing(ctx, m)
	defer cancel()
	return z.client.ListZones(ctx, req)
}

// UpdateZone redibuja una zona existente.
func (z *Zones) UpdateZone(ctx context.Context, m CallMeta, req *zonesv1.UpdateZoneRequest) (*zonesv1.UpdateZoneResponse, error) {
	ctx, cancel := z.outgoing(ctx, m)
	defer cancel()
	return z.client.UpdateZone(ctx, req)
}

// ClassifyPoint devuelve la zona que contiene una coordenada.
func (z *Zones) ClassifyPoint(ctx context.Context, m CallMeta, req *zonesv1.ClassifyPointRequest) (*zonesv1.ClassifyPointResponse, error) {
	ctx, cancel := z.outgoing(ctx, m)
	defer cancel()
	return z.client.ClassifyPoint(ctx, req)
}

// Ready verifica con el protocolo estándar de health de gRPC que Zone Service atiende.
func (z *Zones) Ready(ctx context.Context) bool {
	if z.health == nil {
		return true
	}
	ctx, cancel := context.WithTimeout(ctx, z.timeout)
	defer cancel()
	res, err := z.health.Check(ctx, &healthpb.HealthCheckRequest{Service: zonesv1.ZoneService_ServiceDesc.ServiceName})
	return err == nil && res.GetStatus() == healthpb.HealthCheckResponse_SERVING
}
