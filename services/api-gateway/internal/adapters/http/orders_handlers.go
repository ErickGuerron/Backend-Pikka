package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
)

// OrderClient es lo que los endpoints de pedidos necesitan del Order Service.
type OrderClient interface {
	CreateOrder(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.CreateOrderRequest) (*ordersv1.CreateOrderResponse, error)
	ListMyOrders(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.ListMyOrdersRequest) (*ordersv1.ListMyOrdersResponse, error)
	ListMyNotifications(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.ListMyNotificationsRequest) (*ordersv1.ListMyNotificationsResponse, error)
	ListOrderExceptions(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.ListOrderExceptionsRequest) (*ordersv1.ListOrderExceptionsResponse, error)
	ReviewOrder(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.ReviewOrderRequest) (*ordersv1.ReviewOrderResponse, error)
}

// ordersHandlers traduce REST <-> gRPC. Los roles y el alcance los aplica Order Service;
// aquí solo se propaga la identidad del JWT.
type ordersHandlers struct {
	orders OrderClient
}

// createOrderRequest es el cuerpo de POST /api/v1/orders. El cliente escribe su dirección;
// las coordenadas son opcionales (un punto elegido en el mapa) y, si faltan, el backend geocodifica.
type createOrderRequest struct {
	Address      string   `json:"address"`
	Latitude     *float64 `json:"latitude"`
	Longitude    *float64 `json:"longitude"`
	DeliveryDate string   `json:"deliveryDate"`
	WindowStart  string   `json:"windowStart"`
	WindowEnd    string   `json:"windowEnd"`
}

// reviewOrderRequest es la decisión del operador. El motivo es obligatorio.
type reviewOrderRequest struct {
	Action     string         `json:"action"`
	Reason     string         `json:"reason"`
	Correction *correctionDTO `json:"correction"`
}

// correctionDTO solo lleva los campos que el operador cambia; ausente significa sin cambio.
type correctionDTO struct {
	Address      *string `json:"address"`
	DeliveryDate *string `json:"deliveryDate"`
	WindowStart  *string `json:"windowStart"`
	WindowEnd    *string `json:"windowEnd"`
}

type orderDTO struct {
	ID            string   `json:"id"`
	Address       string   `json:"address"`
	Latitude      float64  `json:"latitude"`
	Longitude     float64  `json:"longitude"`
	DeliveryDate  string   `json:"deliveryDate"`
	WindowStart   string   `json:"windowStart"`
	WindowEnd     string   `json:"windowEnd"`
	Status        string   `json:"status"`
	ReviewReasons []string `json:"reviewReasons"`
	CreatedAt     string   `json:"createdAt"`
}

type notificationDTO struct {
	ID        string `json:"id"`
	OrderID   string `json:"orderId"`
	Kind      string `json:"kind"`
	Detail    string `json:"detail"`
	CreatedAt string `json:"createdAt"`
	Read      bool   `json:"read"`
}

type listOrdersResponse struct {
	Orders []orderDTO `json:"orders"`
}

type listNotificationsResponse struct {
	Notifications []notificationDTO `json:"notifications"`
}

func (h *ordersHandlers) create(c echo.Context) error {
	var req createOrderRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	res, err := h.orders.CreateOrder(c.Request().Context(), callMeta(c), &ordersv1.CreateOrderRequest{
		Address:      req.Address,
		Latitude:     req.Latitude,
		Longitude:    req.Longitude,
		DeliveryDate: req.DeliveryDate,
		WindowStart:  req.WindowStart,
		WindowEnd:    req.WindowEnd,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, toOrderDTO(res.GetOrder()))
}

func (h *ordersHandlers) listMine(c echo.Context) error {
	res, err := h.orders.ListMyOrders(c.Request().Context(), callMeta(c), &ordersv1.ListMyOrdersRequest{})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, listOrdersResponse{Orders: toOrderDTOs(res.GetOrders())})
}

func (h *ordersHandlers) listExceptions(c echo.Context) error {
	res, err := h.orders.ListOrderExceptions(c.Request().Context(), callMeta(c), &ordersv1.ListOrderExceptionsRequest{})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, listOrdersResponse{Orders: toOrderDTOs(res.GetOrders())})
}

func (h *ordersHandlers) review(c echo.Context) error {
	var req reviewOrderRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	action, ok := reviewActionValue(req.Action)
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "action must be CONFIRM, CORRECT or REJECT")
	}
	res, err := h.orders.ReviewOrder(c.Request().Context(), callMeta(c), &ordersv1.ReviewOrderRequest{
		Id:     c.Param("id"),
		Action: action,
		Reason: req.Reason,
		Correction: &ordersv1.OrderCorrection{
			Address:      correctionField(req.Correction, func(v *correctionDTO) *string { return v.Address }),
			DeliveryDate: correctionField(req.Correction, func(v *correctionDTO) *string { return v.DeliveryDate }),
			WindowStart:  correctionField(req.Correction, func(v *correctionDTO) *string { return v.WindowStart }),
			WindowEnd:    correctionField(req.Correction, func(v *correctionDTO) *string { return v.WindowEnd }),
		},
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, toOrderDTO(res.GetOrder()))
}

func (h *ordersHandlers) notifications(c echo.Context) error {
	res, err := h.orders.ListMyNotifications(c.Request().Context(), callMeta(c), &ordersv1.ListMyNotificationsRequest{})
	if err != nil {
		return err
	}
	out := listNotificationsResponse{Notifications: make([]notificationDTO, 0, len(res.GetNotifications()))}
	for _, n := range res.GetNotifications() {
		out.Notifications = append(out.Notifications, notificationDTO{
			ID:        n.GetId(),
			OrderID:   n.GetOrderId(),
			Kind:      n.GetKind(),
			Detail:    n.GetDetail(),
			CreatedAt: n.GetCreatedAt(),
			Read:      n.GetRead(),
		})
	}
	return c.JSON(http.StatusOK, out)
}

// correctionField devuelve el campo de la corrección, o nil si el cliente no lo envió.
func correctionField(c *correctionDTO, pick func(*correctionDTO) *string) *string {
	if c == nil {
		return nil
	}
	return pick(c)
}

func reviewActionValue(action string) (ordersv1.ReviewAction, bool) {
	switch action {
	case "CONFIRM":
		return ordersv1.ReviewAction_REVIEW_ACTION_CONFIRM, true
	case "CORRECT":
		return ordersv1.ReviewAction_REVIEW_ACTION_CORRECT, true
	case "REJECT":
		return ordersv1.ReviewAction_REVIEW_ACTION_REJECT, true
	default:
		return ordersv1.ReviewAction_REVIEW_ACTION_UNSPECIFIED, false
	}
}

func toOrderDTOs(orders []*ordersv1.Order) []orderDTO {
	out := make([]orderDTO, 0, len(orders))
	for _, o := range orders {
		out = append(out, toOrderDTO(o))
	}
	return out
}

func toOrderDTO(o *ordersv1.Order) orderDTO {
	reasons := make([]string, 0, len(o.GetReviewReasons()))
	for _, r := range o.GetReviewReasons() {
		reasons = append(reasons, strings.TrimPrefix(r.String(), "REVIEW_REASON_"))
	}
	return orderDTO{
		ID:            o.GetId(),
		Address:       o.GetAddress(),
		Latitude:      o.GetLatitude(),
		Longitude:     o.GetLongitude(),
		DeliveryDate:  o.GetDeliveryDate(),
		WindowStart:   o.GetWindowStart(),
		WindowEnd:     o.GetWindowEnd(),
		Status:        strings.TrimPrefix(o.GetStatus().String(), "ORDER_STATUS_"),
		ReviewReasons: reasons,
		CreatedAt:     o.GetCreatedAt(),
	}
}
