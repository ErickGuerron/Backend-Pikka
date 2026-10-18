// Package clients adapts Order and Zone Service gRPC clients to the routing ports.
//
// Both services authorize by the caller's identity, so every call forwards the
// identity metadata that the Gateway propagated to the routing request. Routing
// does not invent an identity of its own.
package clients

import (
	"context"

	"google.golang.org/grpc/metadata"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
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
		out = append(out, domain.PlannableOrder{OrderID: order.GetId(), ZoneID: order.GetZoneId(), Address: order.GetAddress()})
	}
	return out, nil
}

// AssignToRoute moves the orders of the stops to ROUTE_ASSIGNED for routeID, all or none.
// It sends stops (with their sequence), not the legacy order_ids.
func (g *OrderGateway) AssignToRoute(ctx context.Context, routeID string, stops []domain.RouteStop) error {
	orderStops := make([]*ordersv1.OrderStop, 0, len(stops))
	for _, stop := range stops {
		orderStops = append(orderStops, &ordersv1.OrderStop{
			OrderId:  stop.OrderID,
			Sequence: int32(stop.Sequence),
		})
	}
	_, err := g.client.AssignOrdersToRoute(forwardIdentity(ctx), &ordersv1.AssignOrdersToRouteRequest{
		RouteId: routeID,
		Stops:   orderStops,
	})
	return err
}

// DriverPool implements ports.DriverPool on top of Driver Service.
type DriverPool struct {
	client driversv1.DriverServiceClient
}

var _ ports.DriverPool = (*DriverPool)(nil)

// NewDriverPool wraps the generated Driver Service client.
func NewDriverPool(client driversv1.DriverServiceClient) *DriverPool {
	return &DriverPool{client: client}
}

// FindAvailable returns the IDs of up to n AVAILABLE drivers, in the order Driver Service returns them.
func (p *DriverPool) FindAvailable(ctx context.Context, n int) ([]string, error) {
	res, err := p.client.FindAvailableDrivers(forwardIdentity(ctx), &driversv1.FindAvailableDriversRequest{Limit: int32(n)})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(res.GetDrivers()))
	for _, driver := range res.GetDrivers() {
		ids = append(ids, driver.GetId())
	}
	return ids, nil
}

// Reserve binds driverID to routeID and returns the driver's full name. Driver Service errors
// (for example FAILED_PRECONDITION when the driver is already reserved) pass through with their gRPC code.
func (p *DriverPool) Reserve(ctx context.Context, driverID, routeID string) (string, error) {
	res, err := p.client.ReserveDriver(forwardIdentity(ctx), &driversv1.ReserveDriverRequest{
		DriverId: driverID,
		RouteId:  routeID,
	})
	if err != nil {
		return "", err
	}
	return res.GetDriver().GetFullName(), nil
}

// Release frees the reservation of routeID held by driverID. Driver Service errors pass
// through with their gRPC code.
func (p *DriverPool) Release(ctx context.Context, driverID, routeID string) error {
	_, err := p.client.ReleaseDriver(forwardIdentity(ctx), &driversv1.ReleaseDriverRequest{
		DriverId: driverID,
		RouteId:  routeID,
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
