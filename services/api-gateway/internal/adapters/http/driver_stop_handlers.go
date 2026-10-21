package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
)

// driverStopHandlers mueve una parada de la ruta del repartidor autenticado. El rol lo fija DriverOnly;
// la pertenencia de la ruta la verifica este handler antes de llamar a Order Service (ADR 0008).
type driverStopHandlers struct {
	drivers DriverClient
	orders  OrderClient
}

type stopStatusRequest struct {
	Status string `json:"status"`
}

type stopStatusResponse struct {
	StopID string `json:"stopId"`
	Status string `json:"status"`
}

// advance responde 200 con el estado nuevo. Solo PICKED_UP e IN_TRANSIT; el resto se rechaza aquí (400)
// porque la entrega y la no entrega van por el registro de entrega, no por este endpoint.
func (h *driverStopHandlers) advance(c echo.Context) error {
	var req stopStatusRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	target, ok := stopTargetFromRequest(req.Status)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "status must be PICKED_UP or IN_TRANSIT")
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
		// Una ruta ajena y una inexistente responden igual: no se revela qué rutas existen.
		return echo.NewHTTPError(http.StatusNotFound, "route not found")
	}

	// El primer PICKED_UP pone al repartidor ON_ROUTE antes de avanzar el pedido. Es idempotente en Driver Service.
	if target == ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP {
		me, err := h.drivers.GetDriverByUserID(ctx, m, &driversv1.GetDriverByUserIDRequest{UserId: id.UserID})
		if err != nil {
			return err
		}
		if _, err := h.drivers.StartRoute(ctx, m, &driversv1.StartRouteRequest{DriverId: me.GetDriver().GetId(), RouteId: routeID}); err != nil {
			return err
		}
	}

	res, err := h.orders.AdvanceRouteStop(ctx, m, &ordersv1.AdvanceRouteStopRequest{
		RouteId:      routeID,
		OrderId:      stopID,
		TargetStatus: target,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, stopStatusResponse{
		StopID: stopID,
		Status: strings.TrimPrefix(res.GetOrder().GetStatus().String(), "ORDER_STATUS_"),
	})
}

// ownsRoute indica si la ruta está reservada para el repartidor del usuario. Una ruta sin repartidor
// (NOT_FOUND) cuenta como no propia, igual que una ajena.
func (h *driverStopHandlers) ownsRoute(ctx context.Context, m grpcclient.CallMeta, userID, routeID string) (bool, error) {
	me, err := h.drivers.GetDriverByUserID(ctx, m, &driversv1.GetDriverByUserIDRequest{UserId: userID})
	if err != nil {
		return false, err
	}
	route, err := h.drivers.GetDriverByRouteID(ctx, m, &driversv1.GetDriverByRouteIDRequest{RouteId: routeID})
	if status.Code(err) == codes.NotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return route.GetDriverId() != "" && route.GetDriverId() == me.GetDriver().GetId(), nil
}

func stopTargetFromRequest(s string) (ordersv1.OrderStatus, bool) {
	switch s {
	case "PICKED_UP":
		return ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP, true
	case "IN_TRANSIT":
		return ordersv1.OrderStatus_ORDER_STATUS_IN_TRANSIT, true
	default:
		return ordersv1.OrderStatus_ORDER_STATUS_UNSPECIFIED, false
	}
}
