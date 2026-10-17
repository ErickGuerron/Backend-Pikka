package http

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/labstack/echo/v4"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
)

// OrderClient es lo que los endpoints de pedidos necesitan del Order Service.
type OrderClient interface {
	CreateOrder(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.CreateOrderRequest) (*ordersv1.CreateOrderResponse, error)
	ListMyOrders(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.ListMyOrdersRequest) (*ordersv1.ListMyOrdersResponse, error)
	ListMyNotifications(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.ListMyNotificationsRequest) (*ordersv1.ListMyNotificationsResponse, error)
	ListOrders(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.ListOrdersRequest) (*ordersv1.ListOrdersResponse, error)
	ListOrderExceptions(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.ListOrderExceptionsRequest) (*ordersv1.ListOrderExceptionsResponse, error)
	ReviewOrder(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.ReviewOrderRequest) (*ordersv1.ReviewOrderResponse, error)
	CancelOrder(ctx context.Context, m grpcclient.CallMeta, req *ordersv1.CancelOrderRequest) (*ordersv1.CancelOrderResponse, error)
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
	// AddressReference y ContactPhone son obligatorios; PostalCode es opcional.
	AddressReference string `json:"addressReference"`
	PostalCode       string `json:"postalCode"`
	ContactPhone     string `json:"contactPhone"`
	// Items son los productos del pedido (ADR 0009). Al menos uno; el precio lo fija el servidor.
	Items []orderItemRequest `json:"items"`
}

type orderItemRequest struct {
	ProductID string `json:"productId"`
	Quantity  int64  `json:"quantity"`
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
	// Vacíos en pedidos anteriores a estos datos: se omiten de la respuesta.
	AddressReference string `json:"addressReference,omitempty"`
	PostalCode       string `json:"postalCode,omitempty"`
	ContactPhone     string `json:"contactPhone,omitempty"`
}

// createdOrderDTO es la respuesta del alta: el pedido más sus productos y su total (ADR 0009).
type createdOrderDTO struct {
	orderDTO
	Items []orderLineDTO `json:"items"`
	Total string         `json:"total"`
}

type orderLineDTO struct {
	ProductID string `json:"productId"`
	Name      string `json:"name"`
	UnitPrice string `json:"unitPrice"`
	Quantity  int64  `json:"quantity"`
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
	reference, postalCode, phone, err := validateOrderContact(req)
	if err != nil {
		return err
	}
	items := make([]*ordersv1.OrderItemRequest, 0, len(req.Items))
	for _, item := range req.Items {
		items = append(items, &ordersv1.OrderItemRequest{ProductId: item.ProductID, Quantity: item.Quantity})
	}
	res, err := h.orders.CreateOrder(c.Request().Context(), callMeta(c), &ordersv1.CreateOrderRequest{
		Address:      req.Address,
		Latitude:     req.Latitude,
		Longitude:    req.Longitude,
		DeliveryDate: req.DeliveryDate,
		WindowStart:  req.WindowStart,
		WindowEnd:    req.WindowEnd,
		Items:        items,

		AddressReference: reference,
		PostalCode:       postalCode,
		ContactPhone:     phone,
	})
	if err != nil {
		return err
	}
	lines := make([]orderLineDTO, 0, len(res.GetItems()))
	for _, l := range res.GetItems() {
		lines = append(lines, orderLineDTO{
			ProductID: l.GetProductId(),
			Name:      l.GetName(),
			UnitPrice: l.GetUnitPrice(),
			Quantity:  l.GetQuantity(),
		})
	}
	return c.JSON(http.StatusCreated, createdOrderDTO{
		orderDTO: toOrderDTO(res.GetOrder()),
		Items:    lines,
		Total:    res.GetTotal(),
	})
}

func (h *ordersHandlers) listMine(c echo.Context) error {
	res, err := h.orders.ListMyOrders(c.Request().Context(), callMeta(c), &ordersv1.ListMyOrdersRequest{})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, listOrdersResponse{Orders: toOrderDTOs(res.GetOrders())})
}

// list devuelve los pedidos de todos los clientes (ADMIN y OPERATOR), con filtros opcionales
// deliveryDate (YYYY-MM-DD) y status. El Order Service valida la fecha y el permiso. Responde un
// arreglo plano, como declara el OpenAPI y espera la web (a diferencia de /orders/exceptions).
func (h *ordersHandlers) list(c echo.Context) error {
	filter, ok := orderStatusFilter(c.QueryParam("status"))
	if !ok {
		return echo.NewHTTPError(http.StatusBadRequest, "status is not a valid order status")
	}
	res, err := h.orders.ListOrders(c.Request().Context(), callMeta(c), &ordersv1.ListOrdersRequest{
		DeliveryDate: strings.TrimSpace(c.QueryParam("deliveryDate")),
		Status:       filter,
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, toOrderDTOs(res.GetOrders()))
}

// orderStatusFilter traduce el parámetro status. Vacío no filtra; un valor desconocido no es válido.
func orderStatusFilter(raw string) (ordersv1.OrderStatus, bool) {
	raw = strings.ToUpper(strings.TrimSpace(raw))
	if raw == "" {
		return ordersv1.OrderStatus_ORDER_STATUS_UNSPECIFIED, true
	}
	if v, ok := ordersv1.OrderStatus_value["ORDER_STATUS_"+raw]; ok && v != 0 {
		return ordersv1.OrderStatus(v), true
	}
	return ordersv1.OrderStatus_ORDER_STATUS_UNSPECIFIED, false
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

// cancel cancela un pedido propio del cliente. Un pedido ajeno llega como NOT_FOUND desde Order Service.
func (h *ordersHandlers) cancel(c echo.Context) error {
	res, err := h.orders.CancelOrder(c.Request().Context(), callMeta(c), &ordersv1.CancelOrderRequest{Id: c.Param("id")})
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

		AddressReference: o.GetAddressReference(),
		PostalCode:       o.GetPostalCode(),
		ContactPhone:     o.GetContactPhone(),
	}
}

var postalCodePattern = regexp.MustCompile(`^[A-Za-z0-9-]{4,10}$`)

// validateOrderContact aplica las reglas del dominio de order-service a los datos de contacto del
// pedido y devuelve los valores normalizados: referencia 3 a 200 caracteres, código postal
// opcional de 4 a 10 letras, dígitos o guion, y teléfono como en el registro.
func validateOrderContact(req createOrderRequest) (reference, postalCode, phone string, err error) {
	reference = strings.TrimSpace(req.AddressReference)
	if n := utf8.RuneCountInString(reference); strings.ContainsRune(reference, '\x00') || n < 3 || n > 200 {
		return "", "", "", echo.NewHTTPError(http.StatusBadRequest, "addressReference must contain 3 to 200 characters")
	}
	postalCode = strings.TrimSpace(req.PostalCode)
	if postalCode != "" && !postalCodePattern.MatchString(postalCode) {
		return "", "", "", echo.NewHTTPError(http.StatusBadRequest, "postalCode must contain 4 to 10 letters, digits or hyphens")
	}
	phone, err = normalizePhone(req.ContactPhone)
	if err != nil {
		return "", "", "", echo.NewHTTPError(http.StatusBadRequest, "contactPhone must have 7 to 15 digits with an optional leading +")
	}
	return reference, postalCode, phone, nil
}
