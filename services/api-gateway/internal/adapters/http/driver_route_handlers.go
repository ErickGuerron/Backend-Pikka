package http

import (
	"net/http"

	"github.com/labstack/echo/v4"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
)

// driverRouteHandlers sirve la ruta del repartidor autenticado. El rol lo fija DriverOnly. La ruta sale
// de su propio perfil en Driver Service, así que nunca se consulta una ruta ajena (ADR 0008).
type driverRouteHandlers struct {
	drivers DriverClient
	orders  OrderClient
	planner RoutePlanner
}

// driverRouteResponse es GET /api/v1/drivers/me/route. Sin ruta: id vacío y paradas vacías (200).
type driverRouteResponse struct {
	ID    string               `json:"id"`
	Stops []driverRouteStopDTO `json:"stops"`
}

// driverRouteStopDTO es una parada para la app del repartidor. Sin datos del cliente: solo dirección,
// ventana, coordenadas y estado. Reason queda en null hasta que exista el resultado de entrega.
type driverRouteStopDTO struct {
	ID          string  `json:"id"`
	Sequence    int32   `json:"sequence"`
	Address     string  `json:"address"`
	WindowStart string  `json:"windowStart"`
	WindowEnd   string  `json:"windowEnd"`
	Status      string  `json:"status"`
	Reason      *string `json:"reason"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
}

// get responde la ruta del repartidor. Un repartidor sin perfil o sin ruta recibe 200 vacío: no tener
// ruta no es un error, y la app muestra su vista de "sin ruta asignada".
func (h *driverRouteHandlers) get(c echo.Context) error {
	ctx := c.Request().Context()
	m := callMeta(c)
	id, _ := identityFrom(c)

	me, err := h.drivers.GetDriverByUserID(ctx, m, &driversv1.GetDriverByUserIDRequest{UserId: id.UserID})
	if status.Code(err) == codes.NotFound {
		return c.JSON(http.StatusOK, emptyDriverRoute())
	}
	if err != nil {
		return err
	}
	routeID := me.GetDriver().GetRouteId()
	if routeID == "" {
		return c.JSON(http.StatusOK, emptyDriverRoute())
	}

	res, err := h.orders.ListDriverRouteStops(ctx, m, &ordersv1.ListDriverRouteStopsRequest{RouteId: routeID})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, driverRouteResponse{ID: routeID, Stops: toDriverRouteStops(res.GetStops())})
}

func emptyDriverRoute() driverRouteResponse {
	return driverRouteResponse{ID: "", Stops: []driverRouteStopDTO{}}
}

func toDriverRouteStops(stops []*ordersv1.DriverRouteStop) []driverRouteStopDTO {
	out := make([]driverRouteStopDTO, 0, len(stops))
	for _, s := range stops {
		out = append(out, driverRouteStopDTO{
			ID:          s.GetOrderId(),
			Sequence:    s.GetSequence(),
			Address:     s.GetAddress(),
			WindowStart: s.GetWindowStart(),
			WindowEnd:   s.GetWindowEnd(),
			Status:      driverRouteStatus(s.GetStatus()),
			Latitude:    s.GetLatitude(),
			Longitude:   s.GetLongitude(),
		})
	}
	return out
}

// driverRouteStatus traduce el estado del pedido al de la parada. ROUTE_ASSIGNED, aún sin pasos del
// conductor, se muestra como PENDING. Cualquier otro estado sin mapeo también se muestra como PENDING.
func driverRouteStatus(s ordersv1.OrderStatus) string {
	switch s {
	case ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP:
		return "PICKED_UP"
	case ordersv1.OrderStatus_ORDER_STATUS_IN_TRANSIT:
		return "IN_TRANSIT"
	case ordersv1.OrderStatus_ORDER_STATUS_DELIVERED:
		return "DELIVERED"
	case ordersv1.OrderStatus_ORDER_STATUS_NOT_DELIVERED:
		return "NOT_DELIVERED"
	default:
		return "PENDING"
	}
}
