// Package clients adapts Order and Zone Service gRPC clients to the routing ports.
//
// Both services authorize by the caller's identity, so every call forwards the
// identity metadata that the Gateway propagated to the routing request. Routing
// does not invent an identity of its own.
package clients

import (
	"context"

	"google.golang.org/grpc/metadata"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	zonesv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/zones/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

// Identity metadata keys propagated by the Gateway (ADR 0002).
const (
	metadataUserID   = "x-user-id"
	metadataUserRole = "x-user-role"
)

// forwardIdentity copies the caller's identity metadata into the outgoing call.
func forwardIdentity(ctx context.Context) context.Context {
	in, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	out := metadata.MD{}
	for _, key := range []string{metadataUserID, metadataUserRole} {
		if values := in.Get(key); len(values) > 0 {
			out.Set(key, values...)
		}
	}
	return metadata.NewOutgoingContext(ctx, out)
}

// OrderGateway implements ports.OrderGateway on top of Order Service.
type OrderGateway struct {
	client ordersv1.OrderServiceClient
}

var _ ports.OrderGateway = (*OrderGateway)(nil)

// NewOrderGateway wraps the generated Order Service client.
func NewOrderGateway(client ordersv1.OrderServiceClient) *OrderGateway {
	return &OrderGateway{client: client}
}

// PendingByDate returns the PENDING orders of a delivery date, mapped to the planner input.
func (g *OrderGateway) PendingByDate(ctx context.Context, deliveryDate string) ([]domain.PlannableOrder, error) {
	res, err := g.client.GetPendingOrders(forwardIdentity(ctx), &ordersv1.GetPendingOrdersRequest{DeliveryDate: deliveryDate})
	if err != nil {
		return nil, err
	}
	out := make([]domain.PlannableOrder, 0, len(res.GetOrders()))
	for _, order := range res.GetOrders() {
		out = append(out, domain.PlannableOrder{OrderID: order.GetId(), ZoneID: order.GetZoneId()})
	}
	return out, nil
}

// AssignToRoute moves the orders to ROUTE_ASSIGNED for routeID, all or none.
func (g *OrderGateway) AssignToRoute(ctx context.Context, routeID string, orderIDs []string) error {
	_, err := g.client.AssignOrdersToRoute(forwardIdentity(ctx), &ordersv1.AssignOrdersToRouteRequest{
		RouteId:  routeID,
		OrderIds: orderIDs,
	})
	return err
}

// ZoneCapacities implements ports.ZoneCapacities on top of Zone Service.
type ZoneCapacities struct {
	client zonesv1.ZoneServiceClient
}

var _ ports.ZoneCapacities = (*ZoneCapacities)(nil)

// NewZoneCapacities wraps the generated Zone Service client.
func NewZoneCapacities(client zonesv1.ZoneServiceClient) *ZoneCapacities {
	return &ZoneCapacities{client: client}
}

// MaxOrdersPerRoute returns the zone's max_orders_per_route setting.
func (z *ZoneCapacities) MaxOrdersPerRoute(ctx context.Context, zoneID string) (int, error) {
	res, err := z.client.GetZone(forwardIdentity(ctx), &zonesv1.GetZoneRequest{Id: zoneID})
	if err != nil {
		return 0, err
	}
	return int(res.GetZone().GetSettings().GetMaxOrdersPerRoute()), nil
}
