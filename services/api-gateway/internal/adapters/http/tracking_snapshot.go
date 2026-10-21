package http

import (
	"context"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	trackingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/tracking/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
)

// Motivos por los que el snapshot no lleva ETA (ADR, customer-live-tracking, supuesto 7).
const (
	etaNoRoute       = "no_route"
	etaNoPosition    = "no_position"
	etaStalePosition = "stale_position"
	etaUnavailable   = "unavailable"
)

// trackingSnapshot es lo único que el cliente ve de su pedido. No lleva identidad del repartidor,
// ruta, otros pedidos ni coordenadas: el id del repartidor solo viaja hacia Tracking Service.
type trackingSnapshot struct {
	OrderID              string         `json:"order_id"`
	Status               string         `json:"status"`
	DeliveryAddress      string         `json:"delivery_address"`
	DeliveryWindow       trackingWindow `json:"delivery_window"`
	ETA                  *trackingETA   `json:"eta"`
	ETAUnavailableReason *string        `json:"eta_unavailable_reason"`
	StopsBefore          *int           `json:"stops_before"`
	// DeliveryPoint es el punto de entrega del pedido del cliente: su propio dato, nunca de otro pedido.
	DeliveryPoint *trackingPoint `json:"delivery_point"`
	// DriverPosition es la última posición fresca del repartidor del pedido, o null. Sin identidad del repartidor.
	DriverPosition *trackingDriverPosition `json:"driver_position"`
	UpdatedAt      string                  `json:"updated_at"`
}

type trackingPoint struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type trackingDriverPosition struct {
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	RecordedAt string  `json:"recorded_at"`
}

type trackingWindow struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type trackingETA struct {
	DistanceMeters  float64 `json:"distance_meters"`
	DurationSeconds float64 `json:"duration_seconds"`
}

// customerTrackingHandlers arma el snapshot de un pedido del cliente y lo sirve por HTTP y WebSocket.
// Los roles y el alcance del pedido los aplica Order Service; aquí solo se compone la vista.
type customerTrackingHandlers struct {
	orders   OrderClient
	drivers  DriverClient
	tracking TrackingClient
	verifier TokenVerifier
	origins  []string
	interval time.Duration
	ticker   func(interval time.Duration) (<-chan time.Time, func())
	now      func() time.Time
	log      *slog.Logger
}

// snapshot responde el estado actual del pedido. Un pedido ajeno o inexistente llega como NOT_FOUND
// (404), con el mismo cuerpo en ambos casos.
func (h *customerTrackingHandlers) snapshot(c echo.Context) error {
	snap, err := h.build(c.Request().Context(), callMeta(c), c.Param("id"))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, snap)
}

// build compone el snapshot. Solo falla si el pedido no se puede leer (propagado tal cual para que el
// error se traduzca a 404 o a 5xx). Las fallas de ruta, paradas o Tracking degradan la vista sin fallar.
func (h *customerTrackingHandlers) build(ctx context.Context, m grpcclient.CallMeta, orderID string) (trackingSnapshot, error) {
	res, err := h.orders.GetMyOrder(ctx, m, &ordersv1.GetMyOrderRequest{OrderId: orderID})
	if err != nil {
		return trackingSnapshot{}, err
	}
	order := res.GetOrder()
	snap := trackingSnapshot{
		OrderID:         order.GetId(),
		Status:          strings.TrimPrefix(order.GetStatus().String(), "ORDER_STATUS_"),
		DeliveryAddress: order.GetAddress(),
		DeliveryWindow:  trackingWindow{Start: order.GetWindowStart(), End: order.GetWindowEnd()},
		StopsBefore:     h.stopsBefore(ctx, m, order),
		DeliveryPoint:   &trackingPoint{Latitude: order.GetLatitude(), Longitude: order.GetLongitude()},
		UpdatedAt:       h.now().UTC().Format(time.RFC3339),
	}
	snap.ETA, snap.ETAUnavailableReason = h.eta(ctx, m, order)
	snap.DriverPosition = h.driverPosition(ctx, m, order)
	return snap, nil
}

// driverPosition pide a Tracking la última posición fresca del repartidor de la ruta. Sin ruta, sin repartidor,
// sin posición fresca o con falla de Tracking devuelve nil: la vista sigue sin mapa, nunca con una posición vieja.
func (h *customerTrackingHandlers) driverPosition(ctx context.Context, m grpcclient.CallMeta, order *ordersv1.Order) *trackingDriverPosition {
	if order.GetRouteId() == "" {
		return nil
	}
	route, err := h.drivers.GetDriverByRouteID(ctx, m, &driversv1.GetDriverByRouteIDRequest{RouteId: order.GetRouteId()})
	if err != nil || route.GetDriverId() == "" {
		return nil
	}
	fix, err := h.tracking.GetDriverPosition(ctx, m, &trackingv1.GetDriverPositionRequest{DriverId: route.GetDriverId()})
	if err != nil {
		if status.Code(err) != codes.NotFound {
			h.log.Warn("tracking: driver position unavailable", "code", status.Code(err).String())
		}
		return nil
	}
	return &trackingDriverPosition{Latitude: fix.GetLatitude(), Longitude: fix.GetLongitude(), RecordedAt: fix.GetRecordedAt()}
}

// stopsBefore pide a Order Service cuántas paradas abiertas quedan antes del pedido del cliente. Sin ruta
// el pedido no tiene paradas todavía, así que es desconocido (nil) y no se consulta. Si el servicio falla,
// también es desconocido: el resto del snapshot se sirve igual.
func (h *customerTrackingHandlers) stopsBefore(ctx context.Context, m grpcclient.CallMeta, order *ordersv1.Order) *int {
	if order.GetRouteId() == "" {
		return nil
	}
	res, err := h.orders.StopsBeforeForOrder(ctx, m, &ordersv1.StopsBeforeForOrderRequest{OrderId: order.GetId()})
	if err != nil {
		h.log.Warn("tracking: stops before unavailable", "code", status.Code(err).String())
		return nil
	}
	n := int(res.GetStopsBefore())
	return &n
}

// eta pide la estimación a Tracking solo con el id del repartidor de la ruta. Nunca inventa un valor: sin
// posición fresca devuelve el motivo y sin ETA.
func (h *customerTrackingHandlers) eta(ctx context.Context, m grpcclient.CallMeta, order *ordersv1.Order) (*trackingETA, *string) {
	if order.GetRouteId() == "" {
		return nil, reason(etaNoRoute)
	}
	route, err := h.drivers.GetDriverByRouteID(ctx, m, &driversv1.GetDriverByRouteIDRequest{RouteId: order.GetRouteId()})
	if status.Code(err) == codes.NotFound || (err == nil && route.GetDriverId() == "") {
		return nil, reason(etaNoRoute)
	}
	if err != nil {
		h.log.Warn("tracking: route driver lookup failed", "code", status.Code(err).String())
		return nil, reason(etaUnavailable)
	}

	est, err := h.tracking.EstimateArrival(ctx, m, &trackingv1.EstimateArrivalRequest{
		DriverId:             route.GetDriverId(),
		DestinationLatitude:  order.GetLatitude(),
		DestinationLongitude: order.GetLongitude(),
	})
	if err == nil {
		return &trackingETA{DistanceMeters: est.GetDistanceMeters(), DurationSeconds: est.GetDurationSeconds()}, nil
	}
	switch status.Code(err) {
	case codes.NotFound:
		return nil, reason(etaNoPosition)
	case codes.FailedPrecondition:
		return nil, reason(etaStalePosition)
	default:
		h.log.Warn("tracking: arrival estimate unavailable", "code", status.Code(err).String())
		return nil, reason(etaUnavailable)
	}
}

func reason(s string) *string { return &s }

// sameView compara dos snapshots ignorando updated_at, que cambia en cada sondeo.
func sameView(a, b trackingSnapshot) bool {
	a.UpdatedAt, b.UpdatedAt = "", ""
	return reflect.DeepEqual(a, b)
}

// defaultTicker es el reloj real de sondeo para cada socket.
func defaultTicker(interval time.Duration) (<-chan time.Time, func()) {
	t := time.NewTicker(interval)
	return t.C, t.Stop
}
