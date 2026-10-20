package http

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"

	"github.com/labstack/echo/v4"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	"github.com/ErickGuerron/Backend-Pikka/libs/osrm"
)

// RoutePlanner ordena las paradas activas por la mejor ruta por calles. Lo implementa el cliente OSRM
// de libs/osrm; el gateway solo lo llama, así que el teléfono nunca habla con OSRM directamente.
type RoutePlanner interface {
	Optimize(ctx context.Context, origin osrm.Point, stops []osrm.Point) (osrm.Trip, error)
}

// driverRoutePlanResponse es GET /api/v1/drivers/me/route/plan. Order trae los ids de las paradas activas en
// el orden óptimo; Geometry es el camino por calles como pares [lat, lon] para dibujarlo en el mapa.
type driverRoutePlanResponse struct {
	Order           []string     `json:"order"`
	DistanceMeters  float64      `json:"distanceMeters"`
	DurationSeconds float64      `json:"durationSeconds"`
	Geometry        [][2]float64 `json:"geometry"`
}

// plan responde el orden óptimo de las paradas activas del repartidor desde su posición (lat, lon de la
// consulta). El origen es un dato personal: no se registra en logs. Sin ruta o sin paradas activas la
// respuesta es vacía y no se llama al planificador.
func (h *driverRouteHandlers) plan(c echo.Context) error {
	origin, err := parsePlanOrigin(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	ctx := c.Request().Context()
	m := callMeta(c)
	id, _ := identityFrom(c)

	me, err := h.drivers.GetDriverByUserID(ctx, m, &driversv1.GetDriverByUserIDRequest{UserId: id.UserID})
	if status.Code(err) == codes.NotFound {
		return c.JSON(http.StatusOK, emptyDriverRoutePlan())
	}
	if err != nil {
		return err
	}
	routeID := me.GetDriver().GetRouteId()
	if routeID == "" {
		return c.JSON(http.StatusOK, emptyDriverRoutePlan())
	}

	res, err := h.orders.ListDriverRouteStops(ctx, m, &ordersv1.ListDriverRouteStopsRequest{RouteId: routeID})
	if err != nil {
		return err
	}
	active := activeStopsInSequence(res.GetStops())
	if len(active) == 0 {
		return c.JSON(http.StatusOK, emptyDriverRoutePlan())
	}
	if h.planner == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "route planning is not configured")
	}

	points := make([]osrm.Point, 0, len(active))
	for _, s := range active {
		points = append(points, osrm.Point{Latitude: s.GetLatitude(), Longitude: s.GetLongitude()})
	}
	trip, err := h.planner.Optimize(ctx, origin, points)
	if errors.Is(err, osrm.ErrNoRoute) || errors.Is(err, osrm.ErrInvalidPoint) {
		return echo.NewHTTPError(http.StatusUnprocessableEntity, "no road route between the driver and the stops")
	}
	if err != nil {
		// Any other failure is the planner being unreachable or unhealthy (OSRM not started, timeout, bad
		// status). The driver app shows a clear "route planner unavailable" for this, not a generic error.
		return echo.NewHTTPError(http.StatusServiceUnavailable, "route planner unavailable")
	}

	order := make([]string, 0, len(trip.Order))
	for _, i := range trip.Order {
		order = append(order, active[i].GetOrderId())
	}
	geometry := make([][2]float64, 0, len(trip.Geometry))
	for _, p := range trip.Geometry {
		geometry = append(geometry, [2]float64{p.Latitude, p.Longitude})
	}
	return c.JSON(http.StatusOK, driverRoutePlanResponse{
		Order:           order,
		DistanceMeters:  trip.DistanceMeters,
		DurationSeconds: trip.DurationSeconds,
		Geometry:        geometry,
	})
}

func emptyDriverRoutePlan() driverRoutePlanResponse {
	return driverRoutePlanResponse{Order: []string{}, Geometry: [][2]float64{}}
}

// parsePlanOrigin lee la posición del repartidor de la consulta. Ambos valores deben ser números y un
// punto válido de la Tierra; cualquier otra cosa es 400 antes de tocar la ruta.
func parsePlanOrigin(c echo.Context) (osrm.Point, error) {
	lat, err := strconv.ParseFloat(c.QueryParam("lat"), 64)
	if err != nil {
		return osrm.Point{}, errors.New("lat must be a number")
	}
	lon, err := strconv.ParseFloat(c.QueryParam("lon"), 64)
	if err != nil {
		return osrm.Point{}, errors.New("lon must be a number")
	}
	p := osrm.Point{Latitude: lat, Longitude: lon}
	if !p.Valid() {
		return osrm.Point{}, errors.New("lat and lon must be a valid coordinate")
	}
	return p, nil
}

// activeStopsInSequence devuelve las paradas que aún no tienen resultado, en el orden de la ruta.
func activeStopsInSequence(stops []*ordersv1.DriverRouteStop) []*ordersv1.DriverRouteStop {
	active := make([]*ordersv1.DriverRouteStop, 0, len(stops))
	for _, s := range stops {
		switch driverRouteStatus(s.GetStatus()) {
		case "DELIVERED", "NOT_DELIVERED":
			continue
		}
		active = append(active, s)
	}
	sort.SliceStable(active, func(a, b int) bool { return active[a].GetSequence() < active[b].GetSequence() })
	return active
}
