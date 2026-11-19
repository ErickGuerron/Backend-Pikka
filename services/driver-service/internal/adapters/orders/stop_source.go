// Package orders obtiene las paradas de una ruta desde Order Service para verificar la llegada.
package orders

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/metadata"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

// identityKeys son las cabeceras que el Gateway propaga y que Order Service exige para leer la ruta.
var identityKeys = []string{"x-request-id", "x-user-id", "x-user-role"}

// StopSource implementa la fuente de paradas con Order Service (ListDriverRouteStops).
type StopSource struct {
	client  ordersv1.OrderServiceClient
	timeout time.Duration
}

// NewStopSource crea la fuente sobre un cliente existente. timeout acota cada llamada.
func NewStopSource(client ordersv1.OrderServiceClient, timeout time.Duration) *StopSource {
	return &StopSource{client: client, timeout: timeout}
}

// Stop devuelve la parada stopID de la ruta routeID. La identidad del repartidor llega en la metadata
// entrante y se reenvia tal cual: Order Service solo lee rutas con rol DRIVER. Una parada que no esta en
// la ruta responde ports.ErrStopNotFound.
func (s *StopSource) Stop(ctx context.Context, routeID, stopID string, at time.Time) (domain.Stop, error) {
	ctx, cancel := context.WithTimeout(forwardIdentity(ctx), s.timeout)
	defer cancel()
	res, err := s.client.ListDriverRouteStops(ctx, &ordersv1.ListDriverRouteStopsRequest{RouteId: routeID})
	if err != nil {
		return domain.Stop{}, err
	}
	for _, row := range res.GetStops() {
		if row.GetOrderId() != stopID {
			continue
		}
		stop, err := domain.NewStop(
			domain.GeoPoint{Latitude: row.GetLatitude(), Longitude: row.GetLongitude()},
			row.GetDeliveryDate(), row.GetWindowStart(), row.GetWindowEnd(), at,
		)
		if err != nil {
			return domain.Stop{}, fmt.Errorf("order service sent an invalid stop: %w", err)
		}
		return stop, nil
	}
	return domain.Stop{}, ports.ErrStopNotFound
}

// forwardIdentity copia a la llamada saliente la identidad y el id de peticion de la entrante.
func forwardIdentity(ctx context.Context) context.Context {
	md, _ := metadata.FromIncomingContext(ctx)
	var pairs []string
	for _, key := range identityKeys {
		for _, value := range md.Get(key) {
			pairs = append(pairs, key, value)
		}
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...)
}
