package http

import (
	"cmp"
	"net/http"
	"slices"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	trackingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/tracking/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/httpx"
)

const noDriverName = "Sin repartidor"

// openRouteStatuses are the order states of a stop that is still to be completed.
var openRouteStatuses = []ordersv1.OrderStatus{
	ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED,
	ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP,
	ordersv1.OrderStatus_ORDER_STATUS_IN_TRANSIT,
}

// trackedRouteDTO is one route in progress for the operator's tracking screen.
type trackedRouteDTO struct {
	ID         string `json:"id"`
	DriverName string `json:"driverName"`
	// DriverPosition is the driver's last fresh position. Omitted when there is none or Tracking is not wired.
	DriverPosition *driverPositionDTO `json:"driverPosition,omitempty"`
	Stops          []trackedStopDTO   `json:"stops"`
}

type driverPositionDTO struct {
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	RecordedAt string  `json:"recordedAt"`
}

type trackedStopDTO struct {
	OrderID  string `json:"orderId"`
	Address  string `json:"address"`
	Sequence int    `json:"sequence"`
	Status   string `json:"status"`
}

// trackingRoutesHandlers serves GET /api/v1/tracking/routes/active (ADMIN and OPERATOR).
type trackingRoutesHandlers struct {
	orders  OrderClient
	drivers DriverClient
	// tracking, when set, adds the driver's last fresh position to each route.
	tracking TrackingClient
}

// active returns the routes that still have an open stop, each with all its stops. It first finds the
// open orders (their routes and delivery days) and then reads only those days, so the cost follows the
// work in progress and not the whole order history.
func (h *trackingRoutesHandlers) active(c *httpx.Context) error {
	ctx := c.Request().Context()
	m := callMeta(c)

	openRoutes := map[string]bool{}
	days := map[string]bool{}
	for _, st := range openRouteStatuses {
		res, err := h.orders.ListOrders(ctx, m, &ordersv1.ListOrdersRequest{Status: st})
		if err != nil {
			return err
		}
		for _, o := range res.GetOrders() {
			if o.GetRouteId() != "" {
				openRoutes[o.GetRouteId()] = true
				days[o.GetDeliveryDate()] = true
			}
		}
	}

	stopsByRoute := map[string][]trackedStopDTO{}
	for day := range days {
		res, err := h.orders.ListOrders(ctx, m, &ordersv1.ListOrdersRequest{DeliveryDate: day})
		if err != nil {
			return err
		}
		for _, o := range res.GetOrders() {
			id := o.GetRouteId()
			if !openRoutes[id] || o.GetStatus() == ordersv1.OrderStatus_ORDER_STATUS_CANCELLED {
				continue
			}
			stopsByRoute[id] = append(stopsByRoute[id], trackedStopDTO{
				OrderID: o.GetId(), Address: o.GetAddress(), Sequence: int(o.GetStopSequence()), Status: driverRouteStatus(o.GetStatus()),
			})
		}
	}

	routeIDs := make([]string, 0, len(stopsByRoute))
	for id, stops := range stopsByRoute {
		slices.SortFunc(stops, func(a, b trackedStopDTO) int {
			return cmp.Or(cmp.Compare(a.Sequence, b.Sequence), cmp.Compare(a.OrderID, b.OrderID))
		})
		routeIDs = append(routeIDs, id)
	}
	slices.Sort(routeIDs)

	out := make([]trackedRouteDTO, 0, len(routeIDs))
	if len(routeIDs) > 0 {
		who, err := h.routeDrivers(c, routeIDs)
		if err != nil {
			return err
		}
		for _, id := range routeIDs {
			out = append(out, trackedRouteDTO{ID: id, DriverName: who[id].name, DriverPosition: h.position(c, who[id].id), Stops: stopsByRoute[id]})
		}
	}
	return c.JSON(http.StatusOK, out)
}

// routeDriver is who runs a route: the driver id (empty when unknown) and the name to show.
type routeDriver struct{ id, name string }

// routeDrivers finds the driver of each route. A route whose driver cannot be resolved shows noDriverName
// instead of failing the whole screen; only the fleet listing failing is an error.
func (h *trackingRoutesHandlers) routeDrivers(c *httpx.Context, routeIDs []string) (map[string]routeDriver, error) {
	ctx := c.Request().Context()
	m := callMeta(c)

	fleet, err := h.drivers.ListDrivers(ctx, m, &driversv1.ListDriversRequest{})
	if err != nil {
		return nil, err
	}
	fullName := map[string]string{}
	for _, d := range fleet.GetDrivers() {
		fullName[d.GetId()] = d.GetFirstName() + " " + d.GetLastName()
	}

	who := make(map[string]routeDriver, len(routeIDs))
	for _, id := range routeIDs {
		who[id] = routeDriver{name: noDriverName}
		r, err := h.drivers.GetDriverByRouteID(ctx, m, &driversv1.GetDriverByRouteIDRequest{RouteId: id})
		if err != nil {
			continue
		}
		if n, ok := fullName[r.GetDriverId()]; ok {
			who[id] = routeDriver{id: r.GetDriverId(), name: n}
		}
	}
	return who, nil
}

// position is the driver's last fresh position, or nil when Tracking is not wired, the driver is unknown or
// there is no recent fix. A Tracking failure hides the marker; it never fails the screen.
func (h *trackingRoutesHandlers) position(c *httpx.Context, driverID string) *driverPositionDTO {
	if h.tracking == nil || driverID == "" {
		return nil
	}
	fix, err := h.tracking.GetDriverPosition(c.Request().Context(), callMeta(c), &trackingv1.GetDriverPositionRequest{DriverId: driverID})
	if err != nil {
		return nil
	}
	return &driverPositionDTO{Latitude: fix.GetLatitude(), Longitude: fix.GetLongitude(), RecordedAt: fix.GetRecordedAt()}
}
