package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	routingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/routing/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
)

// RoutingClient is what the route plan needs from the Routing Service.
type RoutingClient interface {
	GeneratePlan(ctx context.Context, m grpcclient.CallMeta, req *routingv1.GeneratePlanRequest) (*routingv1.GeneratePlanResponse, error)
}

// routePlanHandlers maps POST /api/v1/routes/plan to routing.v1.GeneratePlan. StaffOnly limits the
// route to ADMIN and OPERATOR; the Routing Service validates the role and the date again.
type routePlanHandlers struct {
	routing RoutingClient
}

type generatePlanRequest struct {
	DeliveryDate string `json:"deliveryDate"`
}

type generatePlanResponse struct {
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
func (h *routePlanHandlers) generate(c echo.Context) error {
	var req generatePlanRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	deliveryDate := strings.TrimSpace(req.DeliveryDate)
	if deliveryDate == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "deliveryDate is required")
	}
	res, err := h.routing.GeneratePlan(c.Request().Context(), callMeta(c), &routingv1.GeneratePlanRequest{DeliveryDate: deliveryDate})
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
	unassigned := make([]string, 0, len(res.GetUnassignedOrderIds()))
	unassigned = append(unassigned, res.GetUnassignedOrderIds()...)
	return c.JSON(http.StatusOK, generatePlanResponse{
		PlanID:             res.GetPlanId(),
		Routes:             routes,
		UnassignedOrderIDs: unassigned,
	})
}
