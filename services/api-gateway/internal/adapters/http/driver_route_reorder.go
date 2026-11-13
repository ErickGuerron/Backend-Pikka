package http

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	"github.com/ErickGuerron/Backend-Pikka/libs/osrm"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/httpx"
)

type OrderedRoutePlanner interface {
	InOrder(context.Context, osrm.Point, []osrm.Point) (osrm.Trip, error)
}

type RouteReorderClient interface {
	ReorderDriverRoute(context.Context, grpcclient.CallMeta, *ordersv1.ReorderDriverRouteRequest) (*ordersv1.ReorderDriverRouteResponse, error)
}

type reorderRouteRequest struct {
	FirstStopID string   `json:"firstStopId"`
	Reason      string   `json:"reason"`
	Latitude    *float64 `json:"latitude"`
	Longitude   *float64 `json:"longitude"`
}

const estimatedServiceTime = 5 * time.Minute

func (h *driverRouteHandlers) reorder(c *httpx.Context) error {
	var req reorderRouteRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	reason := strings.TrimSpace(req.Reason)
	length := utf8.RuneCountInString(reason)
	if length < 5 || length > 300 || strings.ContainsRune(reason, 0) || req.Latitude == nil || req.Longitude == nil {
		return httpx.NewHTTPError(http.StatusBadRequest, "location and a reason of 5 to 300 characters are required")
	}
	origin := osrm.Point{Latitude: *req.Latitude, Longitude: *req.Longitude}
	if !origin.Valid() {
		return httpx.NewHTTPError(http.StatusBadRequest, "invalid coordinates")
	}
	ctx, meta := c.Request().Context(), callMeta(c)
	id, _ := identityFrom(c)
	me, err := h.drivers.GetDriverByUserID(ctx, meta, &driversv1.GetDriverByUserIDRequest{UserId: id.UserID})
	if status.Code(err) == codes.NotFound {
		return httpx.NewHTTPError(http.StatusNotFound, "route not found")
	}
	if err != nil {
		return err
	}
	routeID := c.Param("routeId")
	if routeID == "" || routeID != me.GetDriver().GetRouteId() {
		return httpx.NewHTTPError(http.StatusNotFound, "route not found")
	}
	if h.orderedPlanner == nil || h.reorderRoutes == nil || h.timezone == nil {
		return status.Error(codes.Unavailable, "route planner unavailable")
	}
	res, err := h.orders.ListDriverRouteStops(ctx, meta, &ordersv1.ListDriverRouteStopsRequest{RouteId: routeID})
	if err != nil {
		return err
	}
	current := activeStopsInSequence(res.GetStops())
	proposed := make([]*ordersv1.DriverRouteStop, 0, len(current))
	for i, stop := range current {
		if stop.GetOrderId() == req.FirstStopID {
			if i == 0 {
				return c.NoContent(http.StatusNoContent)
			}
			proposed = append(proposed, stop)
			proposed = append(proposed, current[:i]...)
			proposed = append(proposed, current[i+1:]...)
			break
		}
	}
	if len(proposed) == 0 {
		return httpx.NewHTTPError(http.StatusNotFound, "active stop not found")
	}

	now := time.Now()
	beforeETA, err := h.plannedWindows(ctx, now, origin, current)
	if err != nil {
		return err
	}
	afterETA, err := h.plannedWindows(ctx, now, origin, proposed)
	if err != nil {
		return err
	}
	stops := make([]*ordersv1.RouteStopVersion, 0, len(proposed))
	for _, stop := range proposed {
		orderID := stop.GetOrderId()
		if beforeETA[orderID] && !afterETA[orderID] {
			return status.Error(codes.FailedPrecondition, "the proposed order would miss a reachable delivery window")
		}
		stops = append(stops, &ordersv1.RouteStopVersion{OrderId: orderID, Version: stop.GetVersion()})
	}
	if _, err := h.reorderRoutes.ReorderDriverRoute(ctx, meta, &ordersv1.ReorderDriverRouteRequest{RouteId: routeID, Stops: stops, Reason: reason}); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *driverRouteHandlers) plannedWindows(ctx context.Context, now time.Time, origin osrm.Point, stops []*ordersv1.DriverRouteStop) (map[string]bool, error) {
	trip, err := h.orderedPlanner.InOrder(ctx, origin, routeStopPoints(stops))
	if errors.Is(err, osrm.ErrNoRoute) || errors.Is(err, osrm.ErrInvalidPoint) {
		return nil, status.Error(codes.FailedPrecondition, "no road route between the driver and the stops")
	}
	if err != nil {
		return nil, status.Error(codes.Unavailable, "route planner unavailable")
	}
	return windowFeasibility(now, h.timezone, stops, trip.LegSeconds)
}

func routeStopPoints(stops []*ordersv1.DriverRouteStop) []osrm.Point {
	points := make([]osrm.Point, 0, len(stops))
	for _, stop := range stops {
		points = append(points, osrm.Point{Latitude: stop.GetLatitude(), Longitude: stop.GetLongitude()})
	}
	return points
}

func windowFeasibility(now time.Time, timezone *time.Location, stops []*ordersv1.DriverRouteStop, legs []float64) (map[string]bool, error) {
	if len(stops) != len(legs) {
		return nil, status.Error(codes.Unavailable, "incomplete road route")
	}
	feasible := make(map[string]bool, len(stops))
	for i, stop := range stops {
		const layout = "2006-01-02 15:04"
		from, startErr := time.ParseInLocation(layout, stop.GetDeliveryDate()+" "+stop.GetWindowStart(), timezone)
		to, endErr := time.ParseInLocation(layout, stop.GetDeliveryDate()+" "+stop.GetWindowEnd(), timezone)
		if startErr != nil || endErr != nil || !from.Before(to) {
			return nil, status.Error(codes.FailedPrecondition, "a delivery window is invalid")
		}
		now = now.Add(time.Duration(legs[i] * float64(time.Second)))
		if now.Before(from) {
			now = from
		}
		feasible[stop.GetOrderId()] = now.Before(to)
		now = now.Add(estimatedServiceTime)
	}
	return feasible, nil
}
