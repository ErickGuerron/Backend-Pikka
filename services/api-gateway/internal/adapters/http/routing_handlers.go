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
	Routes []plannedRouteDTO `json:"routes"`
}

type plannedRouteDTO struct {
	RouteID    string `json:"routeId"`
	DriverID   string `json:"driverId"`
	OrderCount int32  `json:"orderCount"`
}

// generate responds 200 with the created routes. A date with no orders returns an empty routes array, not null.
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
		routes = append(routes, plannedRouteDTO{
			RouteID:    r.GetRouteId(),
			DriverID:   r.GetDriverId(),
			OrderCount: r.GetOrderCount(),
		})
	}
	return c.JSON(http.StatusOK, generatePlanResponse{Routes: routes})
}
