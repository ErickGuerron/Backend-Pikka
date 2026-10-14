// Package zones adapta Zone Service para que Order Service clasifique direcciones.
package zones

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	zonesv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/zones/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// metadataUserRole es la clave que Zone Service lee para autorizar (ADR 0008).
// "SERVICE" identifica esta llamada interna; no es un rol de usuario.
const (
	metadataUserRole = "x-user-role"
	serviceRole      = "SERVICE"
)

// Classifier implementa ports.ZoneClassifier sobre gRPC.
type Classifier struct {
	client  zonesv1.ZoneServiceClient
	timeout time.Duration
}

var _ ports.ZoneClassifier = (*Classifier)(nil)

// NewClassifier crea el cliente sobre una conexión gRPC existente.
func NewClassifier(conn grpc.ClientConnInterface, timeout time.Duration) *Classifier {
	return &Classifier{client: zonesv1.NewZoneServiceClient(conn), timeout: timeout}
}

// ClassifyPoint devuelve el id de la zona que contiene el punto.
// NotFound del servicio significa fuera de zona; Unavailable o deadline
// significan que no se pudo validar, y el pedido no debe registrarse a ciegas.
func (c *Classifier) ClassifyPoint(ctx context.Context, latitude, longitude float64) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, metadataUserRole, serviceRole)

	res, err := c.client.ClassifyPoint(ctx, &zonesv1.ClassifyPointRequest{
		Point: &zonesv1.Coordinate{Latitude: latitude, Longitude: longitude},
	})
	if err != nil {
		switch status.Code(err) {
		case codes.NotFound:
			return "", domain.ErrZoneNotCovered
		case codes.Unavailable, codes.DeadlineExceeded:
			return "", domain.ErrZonesUnavailable
		default:
			return "", err
		}
	}
	return res.GetZone().GetId(), nil
}
