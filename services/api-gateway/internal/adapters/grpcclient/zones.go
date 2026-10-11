package grpcclient

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	zonesv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/zones/v1"
)

// Zones envuelve al cliente del Zone Service imponiendo deadline y propagando metadata.
type Zones struct {
	client  zonesv1.ZoneServiceClient
	timeout time.Duration
}

// NewZones crea el envoltorio sobre una conexión gRPC.
func NewZones(conn grpc.ClientConnInterface, timeout time.Duration) *Zones {
	return &Zones{client: zonesv1.NewZoneServiceClient(conn), timeout: timeout}
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

// ClassifyPoint devuelve la zona que contiene una coordenada.
func (z *Zones) ClassifyPoint(ctx context.Context, m CallMeta, req *zonesv1.ClassifyPointRequest) (*zonesv1.ClassifyPointResponse, error) {
	ctx, cancel := z.outgoing(ctx, m)
	defer cancel()
	return z.client.ClassifyPoint(ctx, req)
}
