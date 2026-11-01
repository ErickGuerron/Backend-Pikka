package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// covOrders guarda pedidos en memoria y aplica las decisiones como lo hace la transacción real.
type covOrders struct {
	ports.OrderRepository
	orders []domain.Order
}

func (r *covOrders) PlaceOrder(_ context.Context, o domain.Order, _ []domain.OrderLine, _ *domain.Notification, _ *uuid.UUID) (domain.Order, error) {
	o.ID = uuid.NewString()
	r.orders = append(r.orders, o)
	return o, nil
}

func (r *covOrders) ListByCustomer(_ context.Context, customerID uuid.UUID) ([]domain.Order, error) {
	var out []domain.Order
	for _, o := range r.orders {
		if o.CustomerID == customerID.String() {
			out = append(out, o)
		}
	}
	return out, nil
}

func (r *covOrders) ListExceptions(_ context.Context) ([]domain.Order, error) {
	var out []domain.Order
	for _, o := range r.orders {
		if o.Status == domain.StatusPendingReview {
			out = append(out, o)
		}
	}
	return out, nil
}

func (r *covOrders) Review(_ context.Context, orderID string, decide func(domain.Order) (domain.Order, *domain.Notification, error)) (domain.Order, error) {
	for i, o := range r.orders {
		if o.ID != orderID {
			continue
		}
		next, _, err := decide(o)
		if err != nil {
			return domain.Order{}, err
		}
		r.orders[i] = next
		return next, nil
	}
	return domain.Order{}, domain.ErrOrderNotFound
}

// covCustomers resuelve clientes por usuario y por id.
type covCustomers struct {
	ports.CustomerRepository
	byUser map[uuid.UUID]domain.Customer
	byID   map[uuid.UUID]domain.Customer
}

func (c covCustomers) GetByUserID(_ context.Context, userID uuid.UUID) (domain.Customer, error) {
	if cu, ok := c.byUser[userID]; ok {
		return cu, nil
	}
	return domain.Customer{}, domain.ErrCustomerNotFound
}

func (c covCustomers) GetByID(_ context.Context, id uuid.UUID) (domain.Customer, error) {
	if cu, ok := c.byID[id]; ok {
		return cu, nil
	}
	return domain.Customer{}, domain.ErrCustomerNotFound
}

type covZones struct{ zoneID string }

func (z covZones) ClassifyPoint(context.Context, float64, float64) (string, error) {
	return z.zoneID, nil
}

type covGeocoder struct{}

func (covGeocoder) Locate(context.Context, string) (float64, float64, bool, error) {
	return -0.18, -78.48, true, nil
}

type covNotifications struct {
	list []domain.Notification
}

func (n covNotifications) ListByCustomer(context.Context, uuid.UUID) ([]domain.Notification, error) {
	return n.list, nil
}

type covCatalog struct {
	ports.CatalogRepository
	products map[uuid.UUID]domain.Product
}

func (c covCatalog) GetProduct(_ context.Context, id uuid.UUID) (domain.Product, error) {
	if p, ok := c.products[id]; ok {
		return p, nil
	}
	return domain.Product{}, domain.ErrProductNotFound
}

// covFixture arma un OrderServer con casos de uso reales sobre fakes en memoria.
type covFixture struct {
	srv       *OrderServer
	orders    *covOrders
	customer  domain.Customer
	userID    uuid.UUID
	productID uuid.UUID
}

func newCovFixture(t *testing.T, customerStatus domain.CustomerStatus, notices ...domain.Notification) covFixture {
	t.Helper()
	userID := uuid.New()
	cu := domain.NewCustomer(userID, "ana@example.com", "Ana Pérez")
	cu.ID = uuid.New()
	cu.Status = customerStatus
	customers := covCustomers{
		byUser: map[uuid.UUID]domain.Customer{userID: cu},
		byID:   map[uuid.UUID]domain.Customer{cu.ID: cu},
	}
	productID := uuid.New()
	catalog := covCatalog{products: map[uuid.UUID]domain.Product{productID: {
		ID: productID, Name: "Pan", UnitPrice: decimal.RequireFromString("2.50"), Active: true,
	}}}
	orders := &covOrders{}
	loc := time.UTC
	zones := covZones{zoneID: uuid.NewString()}
	uc := OrderUseCases{
		Create:        orderapp.NewCreateOrder(customers, zones, covGeocoder{}, catalog, orders, loc),
		ListMine:      orderapp.NewListMyOrders(customers, orders),
		Exceptions:    orderapp.NewListExceptions(orders),
		Review:        orderapp.NewReviewOrder(customers, zones, covGeocoder{}, orders, loc),
		Notifications: orderapp.NewListMyNotifications(customers, covNotifications{list: notices}),
	}
	return covFixture{srv: NewOrderServer(uc), orders: orders, customer: cu, userID: userID, productID: productID}
}

func covIdentity(userID uuid.UUID, role string) context.Context {
	return identityCtx(userID, role)
}

// identityCtxRaw construye la metadata del Gateway sin validar sus valores, para probar el rechazo.
func identityCtxRaw(userID, role string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(metadataUserID, userID, metadataUserRole, role))
}

func covPendingOrder(t *testing.T, customerID uuid.UUID, reasons ...domain.ReviewReason) domain.Order {
	t.Helper()
	order, err := domain.NewOrder(domain.NewOrderInput{
		Address:      "Avenida Amazonas 123, Quito",
		Latitude:     -0.18,
		Longitude:    -78.48,
		DeliveryDate: "2030-04-12",
		WindowStart:  "09:00",
		WindowEnd:    "12:00",

		AddressReference: "Casa azul", PostalCode: "170150", ContactPhone: "0991234567",
	}, time.Date(2030, 4, 10, 10, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	order.ID = uuid.NewString()
	order.CustomerID = customerID.String()
	order, err = order.ApplyValidation(reasons)
	require.NoError(t, err)
	return order
}

func covCreateReq(productID uuid.UUID) *ordersv1.CreateOrderRequest {
	lat, lng := -0.18, -78.48
	return &ordersv1.CreateOrderRequest{
		Address:          "Avenida Amazonas 123, Quito",
		Latitude:         &lat,
		Longitude:        &lng,
		DeliveryDate:     "2030-04-12",
		WindowStart:      "09:00",
		WindowEnd:        "12:00",
		Items:            []*ordersv1.OrderItemRequest{{ProductId: productID.String(), Quantity: 2}},
		AddressReference: "Casa azul",
		PostalCode:       "170150",
		ContactPhone:     "0991234567",
	}
}

func TestCreateOrderReturnsPlacedOrderWithLinesAndTotal(t *testing.T) {
	fx := newCovFixture(t, domain.StatusActive)

	res, err := fx.srv.CreateOrder(covIdentity(fx.userID, "CUSTOMER"), covCreateReq(fx.productID))

	require.NoError(t, err)
	assert.Equal(t, ordersv1.OrderStatus_ORDER_STATUS_PENDING, res.GetOrder().GetStatus())
	require.Len(t, res.GetItems(), 1)
	assert.Equal(t, fx.productID.String(), res.GetItems()[0].GetProductId())
	assert.Equal(t, "Pan", res.GetItems()[0].GetName())
	assert.Equal(t, "2.50", res.GetItems()[0].GetUnitPrice())
	assert.Equal(t, int64(2), res.GetItems()[0].GetQuantity())
	assert.Equal(t, "5.00", res.GetTotal())
	assert.Equal(t, "0.00", res.GetPendingSurcharge())
	assert.Equal(t, "Casa azul", res.GetOrder().GetAddressReference())
}

func TestCreateOrderRequiresIdentityAndWiring(t *testing.T) {
	fx := newCovFixture(t, domain.StatusActive)

	_, err := fx.srv.CreateOrder(context.Background(), covCreateReq(fx.productID))
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	_, err = NewOrderServer(OrderUseCases{}).CreateOrder(covIdentity(fx.userID, "CUSTOMER"), covCreateReq(fx.productID))
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

func TestCreateOrderRejectsBadInputWithTheRightCode(t *testing.T) {
	fx := newCovFixture(t, domain.StatusActive)
	badProduct := covCreateReq(fx.productID)
	badProduct.Items[0].ProductId = "no-es-uuid"
	noItems := covCreateReq(fx.productID)
	noItems.Items = nil
	unknownProduct := covCreateReq(uuid.New())
	badDate := covCreateReq(fx.productID)
	badDate.DeliveryDate = "2030-13-45"
	badWindow := covCreateReq(fx.productID)
	badWindow.WindowStart, badWindow.WindowEnd = "13:00", "12:00"

	tests := map[string]struct {
		req  *ordersv1.CreateOrderRequest
		want codes.Code
	}{
		"product id is not a UUID": {badProduct, codes.InvalidArgument},
		"order without items":      {noItems, codes.InvalidArgument},
		"unknown product":          {unknownProduct, codes.NotFound},
		"invalid delivery date":    {badDate, codes.InvalidArgument},
		"window ends before start": {badWindow, codes.InvalidArgument},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := fx.srv.CreateOrder(covIdentity(fx.userID, "CUSTOMER"), tt.req)
			assert.Equal(t, tt.want, status.Code(err))
		})
	}
	assert.Empty(t, fx.orders.orders, "ningún intento inválido debe guardar un pedido")
}

func TestCreateOrderBlockedByBalanceIsPermissionDenied(t *testing.T) {
	fx := newCovFixture(t, domain.StatusBlockedSurcharge)

	_, err := fx.srv.CreateOrder(covIdentity(fx.userID, "CUSTOMER"), covCreateReq(fx.productID))

	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.Empty(t, fx.orders.orders)
}

func TestListMyOrdersReturnsOnlyTheCallersOrders(t *testing.T) {
	fx := newCovFixture(t, domain.StatusActive)
	_, err := fx.srv.CreateOrder(covIdentity(fx.userID, "CUSTOMER"), covCreateReq(fx.productID))
	require.NoError(t, err)

	res, err := fx.srv.ListMyOrders(covIdentity(fx.userID, "CUSTOMER"), &ordersv1.ListMyOrdersRequest{})

	require.NoError(t, err)
	require.Len(t, res.GetOrders(), 1)
	assert.Equal(t, "Avenida Amazonas 123, Quito", res.GetOrders()[0].GetAddress())
}

func TestListMyOrdersErrorCodes(t *testing.T) {
	fx := newCovFixture(t, domain.StatusActive)

	_, err := NewOrderServer(OrderUseCases{}).ListMyOrders(covIdentity(fx.userID, "CUSTOMER"), &ordersv1.ListMyOrdersRequest{})
	assert.Equal(t, codes.Unimplemented, status.Code(err))

	_, err = fx.srv.ListMyOrders(context.Background(), &ordersv1.ListMyOrdersRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	_, err = fx.srv.ListMyOrders(covIdentity(fx.userID, "OPERATOR"), &ordersv1.ListMyOrdersRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	_, err = fx.srv.ListMyOrders(covIdentity(uuid.New(), "CUSTOMER"), &ordersv1.ListMyOrdersRequest{})
	assert.Equal(t, codes.NotFound, status.Code(err), "un usuario sin perfil de cliente no tiene pedidos")
}

func TestListMyNotificationsMapsEveryField(t *testing.T) {
	created := time.Date(2030, 4, 10, 15, 0, 0, 0, time.UTC)
	notice := domain.Notification{
		ID: "n-1", OrderID: "o-1", Kind: domain.NotificationRejected, Detail: "Fuera de cobertura",
		CreatedAt: created, Read: true,
	}
	fx := newCovFixture(t, domain.StatusActive, notice)

	res, err := fx.srv.ListMyNotifications(covIdentity(fx.userID, "CUSTOMER"), &ordersv1.ListMyNotificationsRequest{})

	require.NoError(t, err)
	require.Len(t, res.GetNotifications(), 1)
	got := res.GetNotifications()[0]
	assert.Equal(t, "n-1", got.GetId())
	assert.Equal(t, "o-1", got.GetOrderId())
	assert.Equal(t, "ORDER_REJECTED", got.GetKind())
	assert.Equal(t, "Fuera de cobertura", got.GetDetail())
	assert.Equal(t, "2030-04-10T15:00:00Z", got.GetCreatedAt())
	assert.True(t, got.GetRead())
}

func TestListMyNotificationsErrorCodes(t *testing.T) {
	fx := newCovFixture(t, domain.StatusActive)

	_, err := NewOrderServer(OrderUseCases{}).ListMyNotifications(covIdentity(fx.userID, "CUSTOMER"), &ordersv1.ListMyNotificationsRequest{})
	assert.Equal(t, codes.Unimplemented, status.Code(err))

	_, err = fx.srv.ListMyNotifications(context.Background(), &ordersv1.ListMyNotificationsRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	_, err = fx.srv.ListMyNotifications(covIdentity(fx.userID, "ADMIN"), &ordersv1.ListMyNotificationsRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestListOrderExceptionsReturnsOrdersInReviewForOperators(t *testing.T) {
	fx := newCovFixture(t, domain.StatusActive)
	fx.orders.orders = []domain.Order{
		covPendingOrder(t, fx.customer.ID, domain.ReviewZoneNotCovered),
		covPendingOrder(t, fx.customer.ID),
	}

	res, err := fx.srv.ListOrderExceptions(covIdentity(fx.userID, "OPERATOR"), &ordersv1.ListOrderExceptionsRequest{})

	require.NoError(t, err)
	require.Len(t, res.GetOrders(), 1)
	assert.Equal(t, ordersv1.OrderStatus_ORDER_STATUS_PENDING_REVIEW, res.GetOrders()[0].GetStatus())
	assert.Equal(t, []ordersv1.ReviewReason{ordersv1.ReviewReason_REVIEW_REASON_ZONE_NOT_COVERED}, res.GetOrders()[0].GetReviewReasons())
}

func TestListOrderExceptionsErrorCodes(t *testing.T) {
	fx := newCovFixture(t, domain.StatusActive)

	_, err := NewOrderServer(OrderUseCases{}).ListOrderExceptions(covIdentity(fx.userID, "OPERATOR"), &ordersv1.ListOrderExceptionsRequest{})
	assert.Equal(t, codes.Unimplemented, status.Code(err))

	_, err = fx.srv.ListOrderExceptions(context.Background(), &ordersv1.ListOrderExceptionsRequest{})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	_, err = fx.srv.ListOrderExceptions(covIdentity(fx.userID, "CUSTOMER"), &ordersv1.ListOrderExceptionsRequest{})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestReviewOrderConfirmsNonSpatialReviewAndRejectsSpatialOne(t *testing.T) {
	fx := newCovFixture(t, domain.StatusActive)
	nonSpatial := covPendingOrder(t, fx.customer.ID, domain.ReviewCustomerBlocked)
	spatial := covPendingOrder(t, fx.customer.ID, domain.ReviewZoneNotCovered)
	fx.orders.orders = []domain.Order{nonSpatial, spatial}
	op := covIdentity(uuid.New(), "OPERATOR")

	res, err := fx.srv.ReviewOrder(op, &ordersv1.ReviewOrderRequest{
		Id: nonSpatial.ID, Action: ordersv1.ReviewAction_REVIEW_ACTION_CONFIRM, Reason: "Cliente verificado por teléfono",
	})
	require.NoError(t, err)
	assert.Equal(t, ordersv1.OrderStatus_ORDER_STATUS_PENDING, res.GetOrder().GetStatus())

	_, err = fx.srv.ReviewOrder(op, &ordersv1.ReviewOrderRequest{
		Id: spatial.ID, Action: ordersv1.ReviewAction_REVIEW_ACTION_CONFIRM, Reason: "Aceptar igual que está",
	})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestReviewOrderRejectCancelsTheOrder(t *testing.T) {
	fx := newCovFixture(t, domain.StatusActive)
	order := covPendingOrder(t, fx.customer.ID, domain.ReviewZoneNotCovered)
	fx.orders.orders = []domain.Order{order}

	res, err := fx.srv.ReviewOrder(covIdentity(uuid.New(), "ADMIN"), &ordersv1.ReviewOrderRequest{
		Id: order.ID, Action: ordersv1.ReviewAction_REVIEW_ACTION_REJECT, Reason: "Fuera de cobertura confirmada",
	})

	require.NoError(t, err)
	assert.Equal(t, ordersv1.OrderStatus_ORDER_STATUS_CANCELLED, res.GetOrder().GetStatus())
}

func TestReviewOrderCorrectionChangesTheWindow(t *testing.T) {
	fx := newCovFixture(t, domain.StatusActive)
	order := covPendingOrder(t, fx.customer.ID, domain.ReviewInvalidWindow)
	fx.orders.orders = []domain.Order{order}
	end := "13:30"

	res, err := fx.srv.ReviewOrder(covIdentity(uuid.New(), "OPERATOR"), &ordersv1.ReviewOrderRequest{
		Id: order.ID, Action: ordersv1.ReviewAction_REVIEW_ACTION_CORRECT, Reason: "Ventana acordada con el cliente",
		Correction: &ordersv1.OrderCorrection{WindowEnd: &end},
	})

	require.NoError(t, err)
	assert.Equal(t, "13:30", res.GetOrder().GetWindowEnd())
	assert.Equal(t, ordersv1.OrderStatus_ORDER_STATUS_PENDING, res.GetOrder().GetStatus())
}

func TestReviewOrderErrorCodes(t *testing.T) {
	fx := newCovFixture(t, domain.StatusActive)
	order := covPendingOrder(t, fx.customer.ID, domain.ReviewCustomerBlocked)
	fx.orders.orders = []domain.Order{order}
	op := covIdentity(uuid.New(), "OPERATOR")
	confirm := ordersv1.ReviewAction_REVIEW_ACTION_CONFIRM

	_, err := NewOrderServer(OrderUseCases{}).ReviewOrder(op, &ordersv1.ReviewOrderRequest{Id: order.ID, Action: confirm, Reason: "Motivo valido"})
	assert.Equal(t, codes.Unimplemented, status.Code(err))

	_, err = fx.srv.ReviewOrder(context.Background(), &ordersv1.ReviewOrderRequest{Id: order.ID, Action: confirm, Reason: "Motivo valido"})
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	_, err = fx.srv.ReviewOrder(op, &ordersv1.ReviewOrderRequest{Id: order.ID, Action: ordersv1.ReviewAction_REVIEW_ACTION_UNSPECIFIED, Reason: "Motivo valido"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = fx.srv.ReviewOrder(covIdentity(fx.userID, "CUSTOMER"), &ordersv1.ReviewOrderRequest{Id: order.ID, Action: confirm, Reason: "Motivo valido"})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	_, err = fx.srv.ReviewOrder(op, &ordersv1.ReviewOrderRequest{Id: order.ID, Action: confirm, Reason: "x"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err), "el motivo de auditoría es obligatorio")

	_, err = fx.srv.ReviewOrder(op, &ordersv1.ReviewOrderRequest{Id: uuid.NewString(), Action: confirm, Reason: "Motivo valido"})
	assert.Equal(t, codes.NotFound, status.Code(err))

	_, err = fx.srv.ReviewOrder(op, &ordersv1.ReviewOrderRequest{Id: order.ID, Action: ordersv1.ReviewAction_REVIEW_ACTION_CORRECT, Reason: "Motivo valido"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err), "corregir sin campos no hace nada")
}

func TestReviewOrderAfterDecisionIsAborted(t *testing.T) {
	fx := newCovFixture(t, domain.StatusActive)
	order := covPendingOrder(t, fx.customer.ID, domain.ReviewCustomerBlocked)
	fx.orders.orders = []domain.Order{order}
	op := covIdentity(uuid.New(), "OPERATOR")

	_, err := fx.srv.ReviewOrder(op, &ordersv1.ReviewOrderRequest{
		Id: order.ID, Action: ordersv1.ReviewAction_REVIEW_ACTION_REJECT, Reason: "Pedido fuera de servicio",
	})
	require.NoError(t, err)

	_, err = fx.srv.ReviewOrder(op, &ordersv1.ReviewOrderRequest{
		Id: order.ID, Action: ordersv1.ReviewAction_REVIEW_ACTION_CONFIRM, Reason: "Pedido fuera de servicio",
	})
	assert.Equal(t, codes.Aborted, status.Code(err))
}

func TestMapOrderErrorTranslatesDomainAndUseCaseErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"not customer", domain.ErrNotCustomer, codes.PermissionDenied},
		{"not operator", domain.ErrNotOperator, codes.PermissionDenied},
		{"not admin", domain.ErrNotAdmin, codes.PermissionDenied},
		{"invalid delivery date", orderapp.ErrInvalidDeliveryDate, codes.InvalidArgument},
		{"order not found", domain.ErrOrderNotFound, codes.NotFound},
		{"nothing to pay", domain.ErrNothingToPay, codes.FailedPrecondition},
		{"balance changed", domain.ErrBalanceChanged, codes.Aborted},
		{"blocked by surcharge", domain.ErrCustomerBlockedBySurcharge, codes.PermissionDenied},
		{"surcharge not found", domain.ErrSurchargeNotFound, codes.NotFound},
		{"customer not found", domain.ErrCustomerNotFound, codes.NotFound},
		{"customer not recidivist", domain.ErrCustomerNotRecidivist, codes.FailedPrecondition},
		{"dispute window closed", domain.ErrDisputeWindowClosed, codes.FailedPrecondition},
		{"surcharge not disputable", domain.ErrSurchargeNotDisputable, codes.FailedPrecondition},
		{"dispute exists", domain.ErrDisputeExists, codes.Aborted},
		{"dispute already resolved", domain.ErrDisputeAlreadyResolved, codes.Aborted},
		{"dispute not found", domain.ErrDisputeNotFound, codes.NotFound},
		{"unknown dispute decision", domain.ErrUnknownDisputeDecision, codes.InvalidArgument},
		{"reassignment fee not set", domain.ErrReassignmentFeeNotSet, codes.FailedPrecondition},
		{"reassignment charge exists", domain.ErrReassignmentChargeExists, codes.Aborted},
		{"recidivism charge exists", domain.ErrRecidivismChargeExists, codes.Aborted},
		{"invalid recidivism charge", domain.ErrInvalidRecidivismCharge, codes.InvalidArgument},
		{"recidivism charge too high", domain.ErrRecidivismChargeTooHigh, codes.InvalidArgument},
		{"invalid recidivist clear", domain.ErrInvalidRecidivistClear, codes.InvalidArgument},
		{"invalid reassignment fee", domain.ErrInvalidReassignmentFee, codes.InvalidArgument},
		{"invalid reassignment charge", domain.ErrInvalidReassignmentCharge, codes.InvalidArgument},
		{"surcharge not pending verification", domain.ErrSurchargeNotPendingVerification, codes.Aborted},
		{"unknown surcharge decision", domain.ErrUnknownSurchargeDecision, codes.InvalidArgument},
		{"order not in review", domain.ErrOrderNotInReview, codes.Aborted},
		{"order not cancellable", domain.ErrOrderNotCancellable, codes.Aborted},
		{"order not pending", domain.ErrOrderNotPending, codes.Aborted},
		{"invalid transition", domain.ErrInvalidTransition, codes.FailedPrecondition},
		{"not driver", domain.ErrNotDriver, codes.PermissionDenied},
		{"spatial reason not overridable", domain.ErrSpatialReasonNotOverridable, codes.FailedPrecondition},
		{"zones unavailable", domain.ErrZonesUnavailable, codes.Unavailable},
		{"geocoder unavailable", domain.ErrGeocoderUnavailable, codes.Unavailable},
		{"no items", domain.ErrNoItems, codes.InvalidArgument},
		{"invalid quantity", domain.ErrInvalidQuantity, codes.InvalidArgument},
		{"duplicate line", domain.ErrDuplicateLine, codes.InvalidArgument},
		{"product not found", domain.ErrProductNotFound, codes.NotFound},
		{"product inactive", domain.ErrProductInactive, codes.FailedPrecondition},
		{"insufficient stock", domain.ErrInsufficientStock, codes.FailedPrecondition},
		{"reason required", domain.ErrReasonRequired, codes.InvalidArgument},
		{"unknown action", orderapp.ErrUnknownAction, codes.InvalidArgument},
		{"nothing to correct", orderapp.ErrNothingToCorrect, codes.InvalidArgument},
		{"invalid address", domain.ErrInvalidAddress, codes.InvalidArgument},
		{"invalid address reference", domain.ErrInvalidAddressReference, codes.InvalidArgument},
		{"invalid postal code", domain.ErrInvalidPostalCode, codes.InvalidArgument},
		{"invalid phone", domain.ErrInvalidPhone, codes.InvalidArgument},
		{"invalid coordinate", domain.ErrInvalidCoordinate, codes.InvalidArgument},
		{"invalid date", domain.ErrInvalidDate, codes.InvalidArgument},
		{"past delivery date", domain.ErrPastDeliveryDate, codes.InvalidArgument},
		{"invalid window", domain.ErrInvalidWindow, codes.InvalidArgument},
		{"invalid route id", domain.ErrInvalidRouteID, codes.InvalidArgument},
		{"no orders to assign", domain.ErrNoOrdersToAssign, codes.InvalidArgument},
		{"duplicate order id", domain.ErrDuplicateOrderID, codes.InvalidArgument},
		{"invalid order id", domain.ErrInvalidOrderID, codes.InvalidArgument},
		{"invalid stop sequence", domain.ErrInvalidStopSequence, codes.InvalidArgument},
		{"duplicate stop sequence", domain.ErrDuplicateStopSequence, codes.InvalidArgument},
		{"invalid advance target", domain.ErrInvalidAdvanceTarget, codes.InvalidArgument},
		{"invalid stop outcome", domain.ErrInvalidStopOutcome, codes.InvalidArgument},
		{"unexpected failure is internal", errors.New("db down"), codes.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, status.Code(mapOrderError(tt.err)))
		})
	}
}

func TestStatusFromProtoMapsEveryKnownStatus(t *testing.T) {
	known := map[ordersv1.OrderStatus]domain.Status{
		ordersv1.OrderStatus_ORDER_STATUS_PENDING:        domain.StatusPending,
		ordersv1.OrderStatus_ORDER_STATUS_PENDING_REVIEW: domain.StatusPendingReview,
		ordersv1.OrderStatus_ORDER_STATUS_ROUTE_ASSIGNED: domain.StatusRouteAssigned,
		ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP:      domain.StatusPickedUp,
		ordersv1.OrderStatus_ORDER_STATUS_IN_TRANSIT:     domain.StatusInTransit,
		ordersv1.OrderStatus_ORDER_STATUS_DELIVERED:      domain.StatusDelivered,
		ordersv1.OrderStatus_ORDER_STATUS_NOT_DELIVERED:  domain.StatusNotDelivered,
		ordersv1.OrderStatus_ORDER_STATUS_CANCELLED:      domain.StatusCancelled,
	}
	for proto, want := range known {
		got, ok := statusFromProto(proto)
		assert.True(t, ok, proto.String())
		assert.Equal(t, want, got)
		assert.Equal(t, proto, statusToProto(want))
	}
	got, ok := statusFromProto(ordersv1.OrderStatus_ORDER_STATUS_UNSPECIFIED)
	assert.True(t, ok)
	assert.Empty(t, got, "UNSPECIFIED es sin filtro")
	_, ok = statusFromProto(ordersv1.OrderStatus(999))
	assert.False(t, ok, "un valor desconocido no es un filtro válido")
	assert.Equal(t, ordersv1.OrderStatus_ORDER_STATUS_UNSPECIFIED, statusToProto("INVENTADO"))
}

func TestReviewReasonToProtoMapsEveryReason(t *testing.T) {
	tests := map[domain.ReviewReason]ordersv1.ReviewReason{
		domain.ReviewZoneNotCovered:    ordersv1.ReviewReason_REVIEW_REASON_ZONE_NOT_COVERED,
		domain.ReviewInvalidWindow:     ordersv1.ReviewReason_REVIEW_REASON_INVALID_WINDOW,
		domain.ReviewCapacityExceeded:  ordersv1.ReviewReason_REVIEW_REASON_CAPACITY_EXCEEDED,
		domain.ReviewCustomerBlocked:   ordersv1.ReviewReason_REVIEW_REASON_CUSTOMER_BLOCKED,
		domain.ReviewAddressNotLocated: ordersv1.ReviewReason_REVIEW_REASON_ADDRESS_NOT_LOCATED,
		domain.ReviewReason("OTRO"):    ordersv1.ReviewReason_REVIEW_REASON_UNSPECIFIED,
	}
	for reason, want := range tests {
		assert.Equal(t, want, reviewReasonToProto(reason), string(reason))
	}
}

func TestReviewActionFromProtoAcceptsOnlyTheClosedSet(t *testing.T) {
	tests := map[ordersv1.ReviewAction]string{
		ordersv1.ReviewAction_REVIEW_ACTION_CONFIRM: orderapp.ActionConfirm,
		ordersv1.ReviewAction_REVIEW_ACTION_CORRECT: orderapp.ActionCorrect,
		ordersv1.ReviewAction_REVIEW_ACTION_REJECT:  orderapp.ActionReject,
	}
	for proto, want := range tests {
		got, err := reviewActionFromProto(proto)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
	_, err := reviewActionFromProto(ordersv1.ReviewAction_REVIEW_ACTION_UNSPECIFIED)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestCorrectionFromProtoKeepsOnlyTheFieldsSent(t *testing.T) {
	assert.Nil(t, correctionFromProto(nil))

	address := "Calle Germán Alemán 555"
	got := correctionFromProto(&ordersv1.OrderCorrection{Address: &address})

	require.NotNil(t, got)
	require.NotNil(t, got.Address)
	assert.Equal(t, address, *got.Address)
	assert.Nil(t, got.DeliveryDate)
	assert.Nil(t, got.WindowStart)
	assert.Nil(t, got.WindowEnd)
}

func TestOrderLinesToProtoFormatsPricesWithTwoDecimals(t *testing.T) {
	id := uuid.New()
	out := orderLinesToProto([]domain.OrderLine{{ProductID: id, Name: "Pan", UnitPrice: decimal.RequireFromString("2.5"), Quantity: 3}})

	require.Len(t, out, 1)
	assert.Equal(t, id.String(), out[0].GetProductId())
	assert.Equal(t, "2.50", out[0].GetUnitPrice())
	assert.Equal(t, int64(3), out[0].GetQuantity())
}

func TestIdentityFromRejectsMalformedIdentity(t *testing.T) {
	_, _, err := identityFrom(covIdentity(uuid.Nil, "CUSTOMER"))
	assert.NoError(t, err, "uuid.Nil se parsea; el alcance lo decide el caso de uso")

	ctxBadUUID := identityCtxRaw("no-es-uuid", "CUSTOMER")
	_, _, err = identityFrom(ctxBadUUID)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))

	ctxNoRole := identityCtxRaw(uuid.NewString(), "")
	_, _, err = identityFrom(ctxNoRole)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}
