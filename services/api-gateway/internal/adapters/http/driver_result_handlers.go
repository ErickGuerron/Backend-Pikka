package http

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
)

// stopResultRequest es el resultado que el repartidor registra al cerrar una parada. La ubicación GPS
// es obligatoria: Driver Service la compara con la dirección de la parada (ADR 0007, decisión D1).
type stopResultRequest struct {
	Outcome   string   `json:"outcome"`
	Reason    string   `json:"reason"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

// result cierra una parada de la ruta del repartidor autenticado como DELIVERED o NOT_DELIVERED.
// Orden fijo, pensado para no dejar un registro de entrega aceptado sin cierre del pedido:
//  1. la parada debe estar IN_TRANSIT (si no, nada se registra);
//  2. Driver Service registra y verifica la llegada (si rechaza, el pedido no se toca);
//  3. Order Service cierra el pedido.
//
// Si el paso 3 falla después del 2, la respuesta es de error y no se reintenta a ciegas.
func (h *driverStopHandlers) result(c echo.Context) error {
	var req stopResultRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	outcome, ok := stopOutcomeFromRequest(req.Outcome)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "outcome must be DELIVERED or NOT_DELIVERED")
	}
	if req.Latitude == nil || req.Longitude == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "latitude and longitude are required")
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
		return echo.NewHTTPError(http.StatusNotFound, "route not found")
	}

	stops, err := h.orders.ListDriverRouteStops(ctx, m, &ordersv1.ListDriverRouteStopsRequest{RouteId: routeID})
	if err != nil {
		return err
	}
	st, found := stopStatus(stops.GetStops(), stopID)
	if !found {
		return echo.NewHTTPError(http.StatusNotFound, "stop not found")
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
		Latitude:   *req.Latitude,
		Longitude:  *req.Longitude,
	}); err != nil {
		if status.Code(err) == codes.Unimplemented {
			// Driver Service no tiene aún el origen de paradas: el resultado no se registra ni se cierra el pedido.
			return c.JSON(http.StatusNotImplemented, errorBody{Error: APIError{
				Code:      "NOT_IMPLEMENTED",
				Message:   "this driver route is not available yet",
				RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
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
func stopStatus(stops []*ordersv1.DriverRouteStop, stopID string) (ordersv1.OrderStatus, bool) {
	for _, s := range stops {
		if s.GetOrderId() == stopID {
			return s.GetStatus(), true
		}
	}
	return ordersv1.OrderStatus_ORDER_STATUS_UNSPECIFIED, false
}
