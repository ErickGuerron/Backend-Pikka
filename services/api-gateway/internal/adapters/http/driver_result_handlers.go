package http

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/httpx"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
)

// stopResultRequest es el resultado que el repartidor registra al cerrar una parada. La ubicación GPS
// es obligatoria: Driver Service la compara con la dirección de la parada (ADR 0007, decisión D1).
//
// El OpenAPI declara la posición anidada en "location" ({"location":{"latitude","longitude"}}), que es lo
// que envía la app del repartidor. Antes el Gateway solo leía latitude/longitude en la raíz y rechazaba
// toda entrega con 400; se acepta también ese formato plano por compatibilidad.
type stopResultRequest struct {
	Outcome   string        `json:"outcome"`
	Reason    string        `json:"reason"`
	Location  *stopLocation `json:"location"`
	Latitude  *float64      `json:"latitude"`
	Longitude *float64      `json:"longitude"`
}

type stopLocation struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

// position devuelve la ubicación del registro: la de "location" (contrato) o, si falta, la plana.
func (r stopResultRequest) position() (lat, lon float64, ok bool) {
	if r.Location != nil && r.Location.Latitude != nil && r.Location.Longitude != nil {
		return *r.Location.Latitude, *r.Location.Longitude, true
	}
	if r.Latitude != nil && r.Longitude != nil {
		return *r.Latitude, *r.Longitude, true
	}
	return 0, 0, false
}

// result cierra una parada de la ruta del repartidor autenticado como DELIVERED o NOT_DELIVERED.
// Orden fijo, pensado para no dejar un registro de entrega aceptado sin cierre del pedido:
//  1. la parada debe estar IN_TRANSIT (si no, nada se registra);
//  2. Driver Service registra y verifica la llegada (si rechaza, el pedido no se toca);
//  3. Order Service cierra el pedido.
//
// Si el paso 3 falla después del 2, la respuesta es de error y no se reintenta a ciegas.
func (h *driverStopHandlers) result(c *httpx.Context) error {
	var req stopResultRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	outcome, ok := stopOutcomeFromRequest(req.Outcome)
	if !ok {
		return httpx.NewHTTPError(http.StatusBadRequest, "outcome must be DELIVERED or NOT_DELIVERED")
	}
	lat, lon, ok := req.position()
	if !ok {
		return httpx.NewHTTPError(http.StatusBadRequest, "location.latitude and location.longitude are required")
	}

	ctx := c.Request().Context()
	m := callMeta(c)
	id, _ := identityFrom(c)
	routeID, stopID := c.Param("routeId"), c.Param("stopId")

	owned, err := h.ownsRoute(ctx, m, id.UserID, routeID)
	if err != nil {
		return err
	}
	if !owned {
		return httpx.NewHTTPError(http.StatusNotFound, "route not found")
	}

	stops, err := h.orders.ListDriverRouteStops(ctx, m, &ordersv1.ListDriverRouteStopsRequest{RouteId: routeID})
	if err != nil {
		return err
	}
	st, found := stopStatus(stops.GetStops(), stopID)
	if !found {
		return httpx.NewHTTPError(http.StatusNotFound, "stop not found")
	}
	if st != ordersv1.OrderStatus_ORDER_STATUS_IN_TRANSIT {
		return status.Error(codes.FailedPrecondition, "stop must be IN_TRANSIT to record its result")
	}

	me, err := h.drivers.GetDriverByUserID(ctx, m, &driversv1.GetDriverByUserIDRequest{UserId: id.UserID})
	if err != nil {
		return err
	}
	if _, err := h.drivers.RegisterDelivery(ctx, m, &driversv1.RegisterDeliveryRequest{
		DriverId:   me.GetDriver().GetId(),
		RouteId:    routeID,
		StopId:     stopID,
		Outcome:    req.Outcome,
		Reason:     req.Reason,
		RecordedAt: time.Now().UTC().Format(time.RFC3339),
		Latitude:   lat,
		Longitude:  lon,
	}); err != nil {
		if status.Code(err) == codes.Unimplemented {
			// Driver Service no tiene aún el origen de paradas: el resultado no se registra ni se cierra el pedido.
			return c.JSON(http.StatusNotImplemented, errorBody{Error: APIError{
				Code:      "NOT_IMPLEMENTED",
				Message:   "this driver route is not available yet",
				RequestID: c.Response().Header().Get(httpx.HeaderXRequestID),
			}})
		}
		return err
	}

	res, err := h.orders.RecordRouteStopOutcome(ctx, m, &ordersv1.RecordRouteStopOutcomeRequest{
		RouteId: routeID,
		OrderId: stopID,
		Outcome: outcome,
		Reason:  req.Reason,
	})
	if err != nil {
		return err
	}
	h.releaseDriverIfRouteDone(ctx, m, me.GetDriver().GetId(), routeID)
	return c.JSON(http.StatusOK, stopStatusResponse{
		StopID: stopID,
		Status: strings.TrimPrefix(res.GetOrder().GetStatus().String(), "ORDER_STATUS_"),
	})
}

// stopOutcomeFromRequest acepta solo los dos resultados de una parada. Cualquier otro valor se rechaza
// antes de llamar a un servicio.
func stopOutcomeFromRequest(s string) (ordersv1.OrderStatus, bool) {
	switch s {
	case "DELIVERED":
		return ordersv1.OrderStatus_ORDER_STATUS_DELIVERED, true
	case "NOT_DELIVERED":
		return ordersv1.OrderStatus_ORDER_STATUS_NOT_DELIVERED, true
	default:
		return ordersv1.OrderStatus_ORDER_STATUS_UNSPECIFIED, false
	}
}

// stopStatus busca la parada en la ruta del repartidor y devuelve su estado.
// driverReleaser libera la reserva del repartidor (Driver Service ReleaseDriver). El cliente gRPC real lo
// implementa; es opcional para no obligar a cada doble de prueba de DriverClient.
type driverReleaser interface {
	ReleaseDriver(ctx context.Context, m grpcclient.CallMeta, req *driversv1.ReleaseDriverRequest) (*driversv1.ReleaseDriverResponse, error)
}

// releaseDriverIfRouteDone deja al repartidor AVAILABLE cuando la ruta ya no tiene paradas abiertas.
// Antes nadie lo hacía: al cerrar la última parada el repartidor quedaba ON_ROUTE para siempre y la
// planificación nunca encontraba repartidores libres. Es de mejor esfuerzo: la entrega ya quedó
// registrada, así que un fallo aquí se registra en el log y no cambia la respuesta.
func (h *driverStopHandlers) releaseDriverIfRouteDone(ctx context.Context, m grpcclient.CallMeta, driverID, routeID string) {
	releaser, ok := h.drivers.(driverReleaser)
	if !ok || driverID == "" {
		return
	}
	stops, err := h.orders.ListDriverRouteStops(ctx, m, &ordersv1.ListDriverRouteStopsRequest{RouteId: routeID})
	if err != nil {
		slog.Warn("release driver: list route stops failed", "request_id", m.RequestID, "route_id", routeID, "err", err)
		return
	}
	if !routeFinished(stops.GetStops()) {
		return
	}
	if _, err := releaser.ReleaseDriver(ctx, m, &driversv1.ReleaseDriverRequest{DriverId: driverID, RouteId: routeID}); err != nil {
		slog.Warn("release driver: route finished but release failed", "request_id", m.RequestID, "route_id", routeID, "driver_id", driverID, "err", err)
	}
}

// routeFinished es verdadero si todas las paradas tienen un estado final (entregado, no entregado o cancelado).
func routeFinished(stops []*ordersv1.DriverRouteStop) bool {
	for _, s := range stops {
		switch s.GetStatus() {
		case ordersv1.OrderStatus_ORDER_STATUS_DELIVERED, ordersv1.OrderStatus_ORDER_STATUS_NOT_DELIVERED,
			ordersv1.OrderStatus_ORDER_STATUS_CANCELLED:
		default:
			return false
		}
	}
	return true
}

func stopStatus(stops []*ordersv1.DriverRouteStop, stopID string) (ordersv1.OrderStatus, bool) {
	for _, s := range stops {
		if s.GetOrderId() == stopID {
			return s.GetStatus(), true
		}
	}
	return ordersv1.OrderStatus_ORDER_STATUS_UNSPECIFIED, false
}
