package http

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
)

// DriverClient es lo que los endpoints de repartidor necesitan del Driver Service.
type DriverClient interface {
	GetDriverByUserID(ctx context.Context, m grpcclient.CallMeta, req *driversv1.GetDriverByUserIDRequest) (*driversv1.GetDriverByUserIDResponse, error)
	RegisterDelivery(ctx context.Context, m grpcclient.CallMeta, req *driversv1.RegisterDeliveryRequest) (*driversv1.RegisterDeliveryResponse, error)
	GetDriverByRouteID(ctx context.Context, m grpcclient.CallMeta, req *driversv1.GetDriverByRouteIDRequest) (*driversv1.GetDriverByRouteIDResponse, error)
}

// driverRouteNotImplemented responde 501 en las rutas de ruta y parada. Su backend aún no existe:
// el Routing Service posee la ruta y el Tracking Service las transiciones. Nunca se responde 200
// con datos inventados, y el resultado de parada no acepta ubicación de parada del cliente.
func driverRouteNotImplemented(c echo.Context) error {
	return c.JSON(http.StatusNotImplemented, errorBody{Error: APIError{
		Code:      "NOT_IMPLEMENTED",
		Message:   "this driver route is not available yet",
		RequestID: c.Response().Header().Get(echo.HeaderXRequestID),
	}})
}
