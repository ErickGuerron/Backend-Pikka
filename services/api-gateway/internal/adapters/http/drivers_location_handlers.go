package http

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	trackingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/tracking/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
)

// TrackingClient es lo que el endpoint de ubicación necesita del Tracking Service.
type TrackingClient interface {
	RecordPosition(ctx context.Context, m grpcclient.CallMeta, req *trackingv1.RecordPositionRequest) (*trackingv1.RecordPositionResponse, error)
	EstimateArrival(ctx context.Context, m grpcclient.CallMeta, req *trackingv1.EstimateArrivalRequest) (*trackingv1.EstimateArrivalResponse, error)
}

// driverLocationHandlers recibe la posición GPS del repartidor autenticado. El gateway valida la
// forma del cuerpo, resuelve el repartidor con el user id del token y reenvía el fix a Tracking.
type driverLocationHandlers struct {
	drivers  DriverClient
	tracking TrackingClient
}

// driverLocationRequest es el cuerpo de PUT /api/v1/drivers/me/location. Los punteros distinguen
// "ausente" de cero; el rango de coordenadas y la antigüedad los valida Tracking Service.
type driverLocationRequest struct {
	Latitude   *float64 `json:"latitude"`
	Longitude  *float64 `json:"longitude"`
	RecordedAt string   `json:"recordedAt"`
}

// report responde 204 cuando Tracking acepta el fix. Un fix más antiguo que el guardado se descarta
// sin error (contrato de Tracking), por eso también responde 204 en ese caso.
func (h *driverLocationHandlers) report(c echo.Context) error {
	var req driverLocationRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	if req.Latitude == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "latitude is required")
	}
	if req.Longitude == nil {
		return echo.NewHTTPError(http.StatusBadRequest, "longitude is required")
	}
	if req.RecordedAt == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "recordedAt is required")
	}
	if _, err := time.Parse(time.RFC3339, req.RecordedAt); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "recordedAt must be an RFC 3339 timestamp")
	}

	ctx := c.Request().Context()
	id, _ := identityFrom(c)
	driver, err := h.drivers.GetDriverByUserID(ctx, callMeta(c), &driversv1.GetDriverByUserIDRequest{UserId: id.UserID})
	if err != nil {
		return err
	}
	_, err = h.tracking.RecordPosition(ctx, callMeta(c), &trackingv1.RecordPositionRequest{
		DriverId:   driver.GetDriver().GetId(),
		Latitude:   *req.Latitude,
		Longitude:  *req.Longitude,
		RecordedAt: req.RecordedAt,
	})
	if err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}
