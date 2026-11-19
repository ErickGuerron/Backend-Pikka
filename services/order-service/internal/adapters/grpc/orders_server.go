package grpc

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// Metadata que el Gateway propaga tras validar el JWT (ADR 0002).
const (
	metadataUserID   = "x-user-id"
	metadataUserRole = "x-user-role"
)

// OrderUseCases agrupa los casos de uso de pedidos que expone el adaptador gRPC.
type OrderUseCases struct {
	Create        *orderapp.CreateOrder
	ListMine      *orderapp.ListMyOrders
	Exceptions    *orderapp.ListExceptions
	ListAll       *orderapp.ListOrders
	Review        *orderapp.ReviewOrder
	Notifications *orderapp.ListMyNotifications
	Cancel        *orderapp.CancelOrder
	// GetMine lee un pedido del cliente autenticado (GetMyOrder).
	GetMine *orderapp.GetMyOrder
	// RouteAssignment atiende a Routing: pedidos pendientes por fecha y asignación a ruta.
	RouteAssignment *orderapp.RouteAssignment
	// AdvanceStop mueve un pedido de su ruta por PICKED_UP e IN_TRANSIT (AdvanceRouteStop).
	AdvanceStop *orderapp.AdvanceRouteStop
	// StopsBefore cuenta las paradas abiertas antes de un pedido del cliente (StopsBeforeForOrder).
	StopsBefore *orderapp.StopsBeforeForOrder
	// DriverRouteStops lista las paradas de la ruta del conductor (ListDriverRouteStops).
	DriverRouteStops *orderapp.DriverRouteStops
	// RecordOutcome cierra una parada de la ruta del conductor: DELIVERED o NOT_DELIVERED (RecordRouteStopOutcome).
	RecordOutcome *orderapp.RecordStopOutcome
	// Surcharges lets an operator list and resolve surcharges waiting for verification (ADR 0006).
	Surcharges *orderapp.ResolveSurchargeVerification
	// Recidivism lets an operator apply an extra charge to a recidivist customer (ADR 0006).
	Recidivism *orderapp.ApplyRecidivismCharge
	// ListRecidivists lets an operator find the customers an extra charge can be applied to (ADR 0006).
	ListRecidivists *orderapp.ListRecidivists
	// ClearRecidivist lets an administrator remove the reincidence mark (ADR 0006).
	ClearRecidivist *orderapp.ClearRecidivistMark
	// OpenDispute, ListOpenDisputes and ResolveDispute manage the customers' disputes over surcharges (ADR 0007).
	OpenDispute      *orderapp.OpenDispute
	ListOpenDisputes *orderapp.ListOpenDisputes
	ResolveDispute   *orderapp.ResolveDispute
	// SetReassignmentFee, GetReassignmentFee and ApplyReassignmentCharge manage the fixed reassignment fee (ADR 0006).
	SetReassignmentFee      *orderapp.SetReassignmentFee
	GetReassignmentFee      *orderapp.GetReassignmentFee
	ApplyReassignmentCharge *orderapp.ApplyReassignmentCharge
	// Pay lets a customer pay the whole balance with the simulated card (ADR 0006, section 5).
	Pay *orderapp.PayBalance
	// Balance shows a customer what they owe, line by line (ADR 0006, sections 4 and 5).
	Balance *orderapp.GetMyBalance
}

// OrderServer implementa ordersv1.OrderServiceServer. Solo traduce: la regla de
// negocio vive en internal/application/order y en el dominio.
type OrderServer struct {
	ordersv1.UnimplementedOrderServiceServer
	useCases      OrderUseCases
	routeCapacity RouteCapacity
}

// RouteCapacity answers how many orders fit in one driver route of a zone (Zone Service).
type RouteCapacity interface {
	MaxOrdersPerRoute(ctx context.Context, zoneID string) (int, error)
}

// NewOrderServer inyecta los casos de uso.
func NewOrderServer(uc OrderUseCases) *OrderServer {
	return &OrderServer{useCases: uc}
}

// WithRouteCapacity lets the driver's route report the capacity of its zone. Without it max_orders is 0.
func (s *OrderServer) WithRouteCapacity(capacity RouteCapacity) *OrderServer {
	s.routeCapacity = capacity
	return s
}

// CreateOrder registra un pedido del cliente autenticado. Las coordenadas son opcionales.
func (s *OrderServer) CreateOrder(ctx context.Context, req *ordersv1.CreateOrderRequest) (*ordersv1.CreateOrderResponse, error) {
	if s.useCases.Create == nil {
		return nil, status.Error(codes.Unimplemented, "create order is not wired")
	}
	userID, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	items, err := lineRequestsFromProto(req.GetItems())
	if err != nil {
		return nil, err
	}
	placed, err := s.useCases.Create.Place(ctx, orderapp.Command{
		UserID:       userID,
		Role:         role,
		Address:      req.GetAddress(),
		Latitude:     req.Latitude,
		Longitude:    req.Longitude,
		DeliveryDate: req.GetDeliveryDate(),
		WindowStart:  req.GetWindowStart(),
		WindowEnd:    req.GetWindowEnd(),
		Items:        items,

		AddressReference: req.GetAddressReference(),
		PostalCode:       req.GetPostalCode(),
		ContactPhone:     req.GetContactPhone(),
	})
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.CreateOrderResponse{
		Order:            orderToProto(placed.Order),
		Items:            orderLinesToProto(placed.Lines),
		Total:            placed.Total.StringFixed(2),
		PendingSurcharge: placed.PendingSurcharge.StringFixed(2),
	}, nil
}

// orderLinesToProto convierte los productos guardados del pedido al contrato gRPC.
func orderLinesToProto(lines []domain.OrderLine) []*ordersv1.OrderLine {
	out := make([]*ordersv1.OrderLine, 0, len(lines))
	for _, line := range lines {
		out = append(out, &ordersv1.OrderLine{
			ProductId: line.ProductID.String(),
			Name:      line.Name,
			UnitPrice: line.UnitPrice.StringFixed(2),
			Quantity:  line.Quantity,
		})
	}
	return out
}

// ListMyOrders devuelve los pedidos del cliente autenticado.
func (s *OrderServer) ListMyOrders(ctx context.Context, _ *ordersv1.ListMyOrdersRequest) (*ordersv1.ListMyOrdersResponse, error) {
	if s.useCases.ListMine == nil {
		return nil, status.Error(codes.Unimplemented, "list my orders is not wired")
	}
	userID, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	orders, err := s.useCases.ListMine.Run(ctx, orderapp.Command{UserID: userID, Role: role})
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.ListMyOrdersResponse{Orders: ordersToProto(orders)}, nil
}

// ListMyNotifications devuelve los avisos del cliente autenticado.
func (s *OrderServer) ListMyNotifications(ctx context.Context, _ *ordersv1.ListMyNotificationsRequest) (*ordersv1.ListMyNotificationsResponse, error) {
	if s.useCases.Notifications == nil {
		return nil, status.Error(codes.Unimplemented, "list notifications is not wired")
	}
	userID, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	list, err := s.useCases.Notifications.Run(ctx, orderapp.Command{UserID: userID, Role: role})
	if err != nil {
		return nil, mapOrderError(err)
	}
	out := make([]*ordersv1.Notification, 0, len(list))
	for _, n := range list {
		out = append(out, notificationToProto(n))
	}
	return &ordersv1.ListMyNotificationsResponse{Notifications: out}, nil
}

// ListOrders devuelve los pedidos de todos los clientes (solo ADMIN y OPERATOR), con filtros opcionales.
func (s *OrderServer) ListOrders(ctx context.Context, req *ordersv1.ListOrdersRequest) (*ordersv1.ListOrdersResponse, error) {
	if s.useCases.ListAll == nil {
		return nil, status.Error(codes.Unimplemented, "list orders is not wired")
	}
	_, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	filterStatus, ok := statusFromProto(req.GetStatus())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "unrecognized order status")
	}
	orders, err := s.useCases.ListAll.Run(ctx, orderapp.ListOrdersQuery{
		Role: role, DeliveryDate: strings.TrimSpace(req.GetDeliveryDate()), Status: filterStatus,
	})
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.ListOrdersResponse{Orders: ordersToProto(orders)}, nil
}

// ListOrderExceptions devuelve la cola de excepciones (solo ADMIN y OPERATOR).
func (s *OrderServer) ListOrderExceptions(ctx context.Context, _ *ordersv1.ListOrderExceptionsRequest) (*ordersv1.ListOrderExceptionsResponse, error) {
	if s.useCases.Exceptions == nil {
		return nil, status.Error(codes.Unimplemented, "list order exceptions is not wired")
	}
	_, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	orders, err := s.useCases.Exceptions.Run(ctx, role)
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.ListOrderExceptionsResponse{Orders: ordersToProto(orders)}, nil
}

// ReviewOrder aplica la decisión del operador sobre un pedido en revisión.
func (s *OrderServer) ReviewOrder(ctx context.Context, req *ordersv1.ReviewOrderRequest) (*ordersv1.ReviewOrderResponse, error) {
	if s.useCases.Review == nil {
		return nil, status.Error(codes.Unimplemented, "review order is not wired")
	}
	_, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	action, err := reviewActionFromProto(req.GetAction())
	if err != nil {
		return nil, err
	}
	order, err := s.useCases.Review.Run(ctx, orderapp.ReviewCommand{
		Role:       role,
		OrderID:    req.GetId(),
		Action:     action,
		Reason:     req.GetReason(),
		Correction: correctionFromProto(req.GetCorrection()),
	})
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.ReviewOrderResponse{Order: orderToProto(order)}, nil
}

// CancelOrder cancela un pedido del cliente autenticado. Un pedido ajeno responde NOT_FOUND.
func (s *OrderServer) CancelOrder(ctx context.Context, req *ordersv1.CancelOrderRequest) (*ordersv1.CancelOrderResponse, error) {
	if s.useCases.Cancel == nil {
		return nil, status.Error(codes.Unimplemented, "cancel order is not wired")
	}
	userID, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	order, err := s.useCases.Cancel.Run(ctx, orderapp.CancelCommand{
		UserID:  userID,
		Role:    role,
		OrderID: req.GetId(),
	})
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.CancelOrderResponse{Order: orderToProto(order)}, nil
}

// GetMyOrder devuelve un pedido del cliente autenticado. Un pedido ajeno o inexistente responde NOT_FOUND.
func (s *OrderServer) GetMyOrder(ctx context.Context, req *ordersv1.GetMyOrderRequest) (*ordersv1.GetMyOrderResponse, error) {
	if s.useCases.GetMine == nil {
		return nil, status.Error(codes.Unimplemented, "get my order is not wired")
	}
	userID, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	order, err := s.useCases.GetMine.Run(ctx, orderapp.GetCommand{
		UserID:  userID,
		Role:    role,
		OrderID: req.GetOrderId(),
	})
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.GetMyOrderResponse{Order: orderToProto(order)}, nil
}

// identityFrom lee la identidad propagada por el Gateway. Sin ella no hay
// alcance posible, así que la petición se rechaza antes de tocar casos de uso.
func identityFrom(ctx context.Context) (uuid.UUID, string, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	ids := md.Get(metadataUserID)
	roles := md.Get(metadataUserRole)
	if len(ids) == 0 || ids[0] == "" || len(roles) == 0 || roles[0] == "" {
		return uuid.Nil, "", status.Error(codes.Unauthenticated, "missing identity")
	}
	id, err := uuid.Parse(ids[0])
	if err != nil {
		return uuid.Nil, "", status.Error(codes.Unauthenticated, "invalid user identity")
	}
	return id, roles[0], nil
}

// lineRequestsFromProto convierte los productos del pedido. Un id que no es UUID es un error de entrada.
func lineRequestsFromProto(items []*ordersv1.OrderItemRequest) ([]orderapp.LineRequest, error) {
	out := make([]orderapp.LineRequest, 0, len(items))
	for _, item := range items {
		id, err := uuid.Parse(item.GetProductId())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "product_id must be a UUID")
		}
		out = append(out, orderapp.LineRequest{ProductID: id, Quantity: item.GetQuantity()})
	}
	return out, nil
}

func reviewActionFromProto(action ordersv1.ReviewAction) (string, error) {
	switch action {
	case ordersv1.ReviewAction_REVIEW_ACTION_CONFIRM:
		return orderapp.ActionConfirm, nil
	case ordersv1.ReviewAction_REVIEW_ACTION_CORRECT:
		return orderapp.ActionCorrect, nil
	case ordersv1.ReviewAction_REVIEW_ACTION_REJECT:
		return orderapp.ActionReject, nil
	default:
		return "", status.Error(codes.InvalidArgument, "action must be CONFIRM, CORRECT or REJECT")
	}
}

func correctionFromProto(c *ordersv1.OrderCorrection) *orderapp.Correction {
	if c == nil {
		return nil
	}
	return &orderapp.Correction{
		Address:      c.Address,
		DeliveryDate: c.DeliveryDate,
		WindowStart:  c.WindowStart,
		WindowEnd:    c.WindowEnd,
	}
}

// mapOrderError traduce errores de dominio y de caso de uso a códigos gRPC.
// Los mensajes de validación del dominio son seguros para el cliente.
func mapOrderError(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotCustomer), errors.Is(err, domain.ErrNotOperator), errors.Is(err, domain.ErrNotAdmin):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, orderapp.ErrInvalidDeliveryDate):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrOrderNotFound):
		return status.Error(codes.NotFound, "pedido no encontrado")
	case errors.Is(err, domain.ErrNothingToPay):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, domain.ErrBalanceChanged):
		return status.Error(codes.Aborted, err.Error())
	case errors.Is(err, domain.ErrCustomerBlockedBySurcharge):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, domain.ErrSurchargeNotFound):
		return status.Error(codes.NotFound, "recargo no encontrado")
	case errors.Is(err, domain.ErrCustomerNotFound):
		return status.Error(codes.NotFound, "cliente no encontrado")
	case errors.Is(err, domain.ErrCustomerNotRecidivist):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, domain.ErrDisputeWindowClosed), errors.Is(err, domain.ErrSurchargeNotDisputable):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, domain.ErrDisputeExists), errors.Is(err, domain.ErrDisputeAlreadyResolved):
		return status.Error(codes.Aborted, err.Error())
	case errors.Is(err, domain.ErrDisputeNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrUnknownDisputeDecision):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrReassignmentFeeNotSet):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, domain.ErrReassignmentChargeExists), errors.Is(err, domain.ErrRecidivismChargeExists):
		return status.Error(codes.Aborted, err.Error())
	case errors.Is(err, domain.ErrInvalidRecidivismCharge), errors.Is(err, domain.ErrRecidivismChargeTooHigh), errors.Is(err, domain.ErrInvalidRecidivistClear),
		errors.Is(err, domain.ErrInvalidReassignmentFee), errors.Is(err, domain.ErrInvalidReassignmentCharge):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrSurchargeNotPendingVerification):
		return status.Error(codes.Aborted, err.Error())
	case errors.Is(err, domain.ErrUnknownSurchargeDecision):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrOrderNotInReview), errors.Is(err, domain.ErrOrderNotCancellable), errors.Is(err, domain.ErrOrderNotPending):
		// El contrato responde ABORTED (HTTP 409) cuando el estado del pedido no admite la acción.
		return status.Error(codes.Aborted, err.Error())
	case errors.Is(err, domain.ErrInvalidTransition):
		// Paso de ruta ilegal desde el estado actual del pedido (AdvanceRouteStop).
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, domain.ErrNotDriver):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, domain.ErrSpatialReasonNotOverridable):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, domain.ErrZonesUnavailable), errors.Is(err, domain.ErrGeocoderUnavailable):
		return status.Error(codes.Unavailable, "validación de dirección no disponible, intente más tarde")
	case errors.Is(err, domain.ErrNoItems), errors.Is(err, domain.ErrInvalidQuantity), errors.Is(err, domain.ErrDuplicateLine):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrProductNotFound):
		return status.Error(codes.NotFound, "producto no encontrado")
	case errors.Is(err, domain.ErrProductInactive), errors.Is(err, domain.ErrInsufficientStock):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, domain.ErrReasonRequired),
		errors.Is(err, orderapp.ErrUnknownAction),
		errors.Is(err, orderapp.ErrNothingToCorrect),
		errors.Is(err, domain.ErrInvalidAddress),
		errors.Is(err, domain.ErrInvalidAddressReference),
		errors.Is(err, domain.ErrInvalidPostalCode),
		errors.Is(err, domain.ErrInvalidPhone),
		errors.Is(err, domain.ErrInvalidCoordinate),
		errors.Is(err, domain.ErrInvalidDate),
		errors.Is(err, domain.ErrPastDeliveryDate),
		errors.Is(err, domain.ErrInvalidWindow),
		errors.Is(err, domain.ErrInvalidRouteID),
		errors.Is(err, domain.ErrNoOrdersToAssign),
		errors.Is(err, domain.ErrDuplicateOrderID),
		errors.Is(err, domain.ErrInvalidOrderID),
		errors.Is(err, domain.ErrInvalidStopSequence),
		errors.Is(err, domain.ErrDuplicateStopSequence),
		errors.Is(err, domain.ErrInvalidAdvanceTarget),
		errors.Is(err, domain.ErrInvalidStopOutcome):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return mapError(err)
	}
}

func ordersToProto(orders []domain.Order) []*ordersv1.Order {
	out := make([]*ordersv1.Order, 0, len(orders))
	for _, order := range orders {
		out = append(out, orderToProto(order))
	}
	return out
}

// orderToProto traduce el dominio al mensaje del contrato.
func orderToProto(o domain.Order) *ordersv1.Order {
	reasons := make([]ordersv1.ReviewReason, 0, len(o.ReviewReasons))
	for _, reason := range o.ReviewReasons {
		reasons = append(reasons, reviewReasonToProto(reason))
	}
	return &ordersv1.Order{
		Id:            o.ID,
		Address:       o.Address,
		Latitude:      o.Latitude,
		Longitude:     o.Longitude,
		DeliveryDate:  o.DeliveryDate.String(),
		WindowStart:   o.Window.Start(),
		WindowEnd:     o.Window.End(),
		Status:        statusToProto(o.Status),
		ReviewReasons: reasons,
		CreatedAt:     o.CreatedAt.UTC().Format(time.RFC3339),
		ZoneId:        o.ZoneID,
		RouteId:       o.RouteID,
		QueuePosition: int32(o.QueuePosition),

		AddressReference: o.AddressReference,
		PostalCode:       o.PostalCode,
		ContactPhone:     o.ContactPhone,
	}
}

func notificationToProto(n domain.Notification) *ordersv1.Notification {
	return &ordersv1.Notification{
		Id:        n.ID,
		OrderId:   n.OrderID,
		Kind:      string(n.Kind),
		Detail:    n.Detail,
		CreatedAt: n.CreatedAt.UTC().Format(time.RFC3339),
		Read:      n.Read,
	}
}

// statusFromProto traduce el filtro del contrato. UNSPECIFIED es "sin filtro"; un valor que el
// servicio no conoce no es válido.
func statusFromProto(s ordersv1.OrderStatus) (domain.Status, bool) {
	if s == ordersv1.OrderStatus_ORDER_STATUS_UNSPECIFIED {
		return "", true
	}
	for _, candidate := range []domain.Status{
		domain.StatusPending, domain.StatusPendingReview, domain.StatusRouteAssigned, domain.StatusPickedUp,
		domain.StatusInTransit, domain.StatusDelivered, domain.StatusNotDelivered, domain.StatusCancelled,
	} {
		if statusToProto(candidate) == s {
			return candidate, true
		}
	}
	return "", false
}

func statusToProto(s domain.Status) ordersv1.OrderStatus {
	switch s {
	case domain.StatusPending:
		return ordersv1.OrderStatus_ORDER_STATUS_PENDING
	case domain.StatusPendingReview:
		return ordersv1.OrderStatus_ORDER_STATUS_PENDING_REVIEW
	case domain.StatusRouteAssigned:
		return ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED
	case domain.StatusPickedUp:
		return ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP
	case domain.StatusInTransit:
		return ordersv1.OrderStatus_ORDER_STATUS_IN_TRANSIT
	case domain.StatusDelivered:
		return ordersv1.OrderStatus_ORDER_STATUS_DELIVERED
	case domain.StatusNotDelivered:
		return ordersv1.OrderStatus_ORDER_STATUS_NOT_DELIVERED
	case domain.StatusCancelled:
		return ordersv1.OrderStatus_ORDER_STATUS_CANCELLED
	default:
		return ordersv1.OrderStatus_ORDER_STATUS_UNSPECIFIED
	}
}

func reviewReasonToProto(r domain.ReviewReason) ordersv1.ReviewReason {
	switch r {
	case domain.ReviewZoneNotCovered:
		return ordersv1.ReviewReason_REVIEW_REASON_ZONE_NOT_COVERED
	case domain.ReviewInvalidWindow:
		return ordersv1.ReviewReason_REVIEW_REASON_INVALID_WINDOW
	case domain.ReviewCapacityExceeded:
		return ordersv1.ReviewReason_REVIEW_REASON_CAPACITY_EXCEEDED
	case domain.ReviewCustomerBlocked:
		return ordersv1.ReviewReason_REVIEW_REASON_CUSTOMER_BLOCKED
	case domain.ReviewAddressNotLocated:
		return ordersv1.ReviewReason_REVIEW_REASON_ADDRESS_NOT_LOCATED
	default:
		return ordersv1.ReviewReason_REVIEW_REASON_UNSPECIFIED
	}
}
