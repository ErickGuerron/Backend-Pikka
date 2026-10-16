package grpcclient

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	trackingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/tracking/v1"
)

// Tracking envuelve al cliente del Tracking Service para el reporte de posición del repartidor.
// Impone deadline y propaga la identidad del Gateway, igual que Drivers.
type Tracking struct {
	client  trackingv1.TrackingServiceClient
	timeout time.Duration
}

// NewTracking crea el envoltorio sobre una conexión gRPC existente.
func NewTracking(conn grpc.ClientConnInterface, timeout time.Duration) *Tracking {
	return &Tracking{client: trackingv1.NewTrackingServiceClient(conn), timeout: timeout}
}

// NewTrackingWithClient permite inyectar un cliente falso en pruebas.
func NewTrackingWithClient(client trackingv1.TrackingServiceClient, timeout time.Duration) *Tracking {
	return &Tracking{client: client, timeout: timeout}
}

func (t *Tracking) outgoing(ctx context.Context, m CallMeta) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	pairs := []string{MetadataRequestID, m.RequestID}
	if m.UserID != "" {
		pairs = append(pairs, MetadataUserID, m.UserID, MetadataUserRole, m.Role)
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...), cancel
}

// RecordPosition envía un fix del repartidor al Tracking Service. La posición no se registra en logs.
func (t *Tracking) RecordPosition(ctx context.Context, m CallMeta, req *trackingv1.RecordPositionRequest) (*trackingv1.RecordPositionResponse, error) {
	ctx, cancel := t.outgoing(ctx, m)
	defer cancel()
	return t.client.RecordPosition(ctx, req)
}
