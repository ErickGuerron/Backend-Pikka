package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/httpx"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	routingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/routing/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
)

// RoutingClient is what the route plan needs from the Routing Service.
type RoutingClient interface {
	GeneratePlan(ctx context.Context, m grpcclient.CallMeta, req *routingv1.GeneratePlanRequest) (*routingv1.GeneratePlanResponse, error)
	AddOrderToRoute(ctx context.Context, m grpcclient.CallMeta, req *routingv1.AddOrderToRouteRequest) (*routingv1.AddOrderToRouteResponse, error)
}

// routePlanHandlers maps POST /api/v1/routes/plan to routing.v1.GeneratePlan. StaffOnly limits the
// route to ADMIN and OPERATOR; the Routing Service validates the role and the date again.
type routePlanHandlers struct {
	routing RoutingClient
	// drivers, when set, names the drivers the Routing Service only identifies (a dry run reserves nobody).
	drivers DriverClient
}

type generatePlanRequest struct {
	DeliveryDate string `json:"deliveryDate"`
	// DryRun proposes the plan without reserving drivers or assigning orders.
	DryRun bool `json:"dryRun"`
}

type generatePlanResponse struct {
	// DryRun is true when the plan is only a proposal: nothing was reserved or assigned. Omitted otherwise.
	DryRun             bool              `json:"dryRun,omitempty"`
	PlanID             string            `json:"planId"`
	Routes             []plannedRouteDTO `json:"routes"`
	UnassignedOrderIDs []string          `json:"unassignedOrderIds"`
}

type plannedRouteDTO struct {
	RouteID    string           `json:"routeId"`
	DriverID   string           `json:"driverId"`
	DriverName string           `json:"driverName"`
	OrderCount int32            `json:"orderCount"`
	Stops      []plannedStopDTO `json:"stops"`
}

type plannedStopDTO struct {
	OrderID  string `json:"orderId"`
	Address  string `json:"address"`
	Sequence int32  `json:"sequence"`
}

// generate responds 200 with the created routes. A date with no orders returns an empty routes array, not null.
// planId and unassignedOrderIds are phase-1 values: the plan id is derived and not persisted, and the
// unassigned list is always empty. Both are always arrays or strings, never null.
func (h *routePlanHandlers) generate(c *httpx.Context) error {
	var req generatePlanRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	deliveryDate := strings.TrimSpace(req.DeliveryDate)
	if deliveryDate == "" {
		return httpx.NewHTTPError(http.StatusBadRequest, "deliveryDate is required")
	}
	res, err := h.routing.GeneratePlan(c.Request().Context(), callMeta(c), &routingv1.GeneratePlanRequest{DeliveryDate: deliveryDate, DryRun: req.DryRun})
	if err != nil {
		return err
	}
	routes := make([]plannedRouteDTO, 0, len(res.GetRoutes()))
	for _, r := range res.GetRoutes() {
		stops := make([]plannedStopDTO, 0, len(r.GetStops()))
		for _, s := range r.GetStops() {
			stops = append(stops, plannedStopDTO{OrderID: s.GetOrderId(), Address: s.GetAddress(), Sequence: s.GetSequence()})
		}
		routes = append(routes, plannedRouteDTO{
			RouteID:    r.GetRouteId(),
			DriverID:   r.GetDriverId(),
			DriverName: r.GetDriverName(),
			OrderCount: r.GetOrderCount(),
			Stops:      stops,
		})
	}
	h.nameDrivers(c, routes)
	unassigned := make([]string, 0, len(res.GetUnassignedOrderIds()))
	unassigned = append(unassigned, res.GetUnassignedOrderIds()...)
	return c.JSON(http.StatusOK, generatePlanResponse{
		DryRun:             req.DryRun,
		PlanID:             res.GetPlanId(),
		Routes:             routes,
		UnassignedOrderIDs: unassigned,
	})
}

// nameDrivers fills the name of every route whose driver the Routing Service identified but did not
// name (a dry run reserves nobody). It is best effort: without the fleet the names stay empty.
func (h *routePlanHandlers) nameDrivers(c *httpx.Context, routes []plannedRouteDTO) {
	if h.drivers == nil {
		return
	}
	needs := false
	for _, r := range routes {
		needs = needs || (r.DriverID != "" && r.DriverName == "")
	}
	if !needs {
		return
	}
	fleet, err := h.drivers.ListDrivers(c.Request().Context(), callMeta(c), &driversv1.ListDriversRequest{})
	if err != nil {
		return
	}
	names := make(map[string]string, len(fleet.GetDrivers()))
	for _, d := range fleet.GetDrivers() {
		names[d.GetId()] = strings.TrimSpace(d.GetFirstName() + " " + d.GetLastName())
	}
	for i := range routes {
		if routes[i].DriverName == "" {
			routes[i].DriverName = names[routes[i].DriverID]
		}
	}
}

type addOrderToRouteRequest struct {
	OrderID string `json:"orderId"`
}

type addOrderToRouteResponse struct {
	RouteID string         `json:"routeId"`
	Stop    plannedStopDTO `json:"stop"`
}

// addOrder maps POST /api/v1/routes/:routeId/orders to routing.v1.AddOrderToRoute: it puts a PENDING
// order at the end of a route that is still running.
func (h *routePlanHandlers) addOrder(c *httpx.Context) error {
	var req addOrderToRouteRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	orderID := strings.TrimSpace(req.OrderID)
	if orderID == "" {
		return httpx.NewHTTPError(http.StatusBadRequest, "orderId is required")
	}
	res, err := h.routing.AddOrderToRoute(c.Request().Context(), callMeta(c), &routingv1.AddOrderToRouteRequest{
		RouteId: c.Param("routeId"),
		OrderId: orderID,
	})
	if err != nil {
		return err
	}
	stop := res.GetStop()
	return c.JSON(http.StatusCreated, addOrderToRouteResponse{
		RouteID: res.GetRouteId(),
		Stop:    plannedStopDTO{OrderID: stop.GetOrderId(), Address: stop.GetAddress(), Sequence: stop.GetSequence()},
	})
}
