package grpcclient

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
)

// Orders envuelve al cliente del Order Service para los endpoints de pedidos.
// Impone deadline y propaga la identidad del Gateway, igual que Customer.
type Orders struct {
	client  ordersv1.OrderServiceClient
	timeout time.Duration
}

// NewOrders crea el envoltorio sobre una conexión gRPC existente.
func NewOrders(conn grpc.ClientConnInterface, timeout time.Duration) *Orders {
	return &Orders{client: ordersv1.NewOrderServiceClient(conn), timeout: timeout}
}

// NewOrdersWithClient permite inyectar un cliente falso en pruebas.
func NewOrdersWithClient(client ordersv1.OrderServiceClient, timeout time.Duration) *Orders {
	return &Orders{client: client, timeout: timeout}
}

func (o *Orders) outgoing(ctx context.Context, m CallMeta) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	pairs := []string{MetadataRequestID, m.RequestID}
	if m.UserID != "" {
		pairs = append(pairs, MetadataUserID, m.UserID, MetadataUserRole, m.Role)
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...), cancel
}

// CreateOrder registra un pedido del cliente autenticado.
func (o *Orders) CreateOrder(ctx context.Context, m CallMeta, req *ordersv1.CreateOrderRequest) (*ordersv1.CreateOrderResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.CreateOrder(ctx, req)
}

// ListMyOrders devuelve los pedidos del cliente autenticado.
func (o *Orders) ListMyOrders(ctx context.Context, m CallMeta, req *ordersv1.ListMyOrdersRequest) (*ordersv1.ListMyOrdersResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.ListMyOrders(ctx, req)
}

// ListMyNotifications devuelve los avisos del cliente autenticado.
func (o *Orders) ListMyNotifications(ctx context.Context, m CallMeta, req *ordersv1.ListMyNotificationsRequest) (*ordersv1.ListMyNotificationsResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.ListMyNotifications(ctx, req)
}

// ListOrderExceptions devuelve la cola de excepciones (ADMIN y OPERATOR).
func (o *Orders) ListOrderExceptions(ctx context.Context, m CallMeta, req *ordersv1.ListOrderExceptionsRequest) (*ordersv1.ListOrderExceptionsResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.ListOrderExceptions(ctx, req)
}

// ReviewOrder aplica la decisión del operador sobre un pedido en revisión.
func (o *Orders) ReviewOrder(ctx context.Context, m CallMeta, req *ordersv1.ReviewOrderRequest) (*ordersv1.ReviewOrderResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.ReviewOrder(ctx, req)
}

// ListSurchargesPendingVerification devuelve la cola de recargos por verificar (ADMIN y OPERATOR).
func (o *Orders) ListSurchargesPendingVerification(ctx context.Context, m CallMeta, req *ordersv1.ListSurchargesPendingVerificationRequest) (*ordersv1.ListSurchargesPendingVerificationResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.ListSurchargesPendingVerification(ctx, req)
}

// ResolveSurchargeVerification confirma o dispensa un recargo en verificacion.
func (o *Orders) ResolveSurchargeVerification(ctx context.Context, m CallMeta, req *ordersv1.ResolveSurchargeVerificationRequest) (*ordersv1.ResolveSurchargeVerificationResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.ResolveSurchargeVerification(ctx, req)
}

// ApplyRecidivismCharge aplica el recargo extra del operador a un cliente reincidente.
func (o *Orders) ApplyRecidivismCharge(ctx context.Context, m CallMeta, req *ordersv1.ApplyRecidivismChargeRequest) (*ordersv1.ApplyRecidivismChargeResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.ApplyRecidivismCharge(ctx, req)
}

// ClearRecidivist quita la marca de reincidente de un cliente (solo ADMIN).
func (o *Orders) ClearRecidivist(ctx context.Context, m CallMeta, req *ordersv1.ClearRecidivistRequest) (*ordersv1.ClearRecidivistResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.ClearRecidivist(ctx, req)
}

// ListRecidivists lista los clientes con la marca de reincidente para el operador.
func (o *Orders) ListRecidivists(ctx context.Context, m CallMeta, req *ordersv1.ListRecidivistsRequest) (*ordersv1.ListRecidivistsResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.ListRecidivists(ctx, req)
}

// PayMyBalance paga el saldo completo del cliente autenticado con el cobro simulado.
func (o *Orders) PayMyBalance(ctx context.Context, m CallMeta, req *ordersv1.PayMyBalanceRequest) (*ordersv1.PayMyBalanceResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.PayMyBalance(ctx, req)
}

// GetMyBalance muestra al cliente autenticado lo que debe, linea por linea.
func (o *Orders) GetMyBalance(ctx context.Context, m CallMeta, req *ordersv1.GetMyBalanceRequest) (*ordersv1.GetMyBalanceResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.GetMyBalance(ctx, req)
}

// CancelOrder cancela un pedido del cliente autenticado.
func (o *Orders) CancelOrder(ctx context.Context, m CallMeta, req *ordersv1.CancelOrderRequest) (*ordersv1.CancelOrderResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.CancelOrder(ctx, req)
}

// GetMyOrder lee un pedido del cliente autenticado. Un pedido ajeno o inexistente llega como NOT_FOUND.
func (o *Orders) GetMyOrder(ctx context.Context, m CallMeta, req *ordersv1.GetMyOrderRequest) (*ordersv1.GetMyOrderResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.GetMyOrder(ctx, req)
}

// StopsBeforeForOrder cuenta las paradas abiertas antes de un pedido propio del cliente. Un pedido ajeno
// o inexistente llega como NOT_FOUND.
func (o *Orders) StopsBeforeForOrder(ctx context.Context, m CallMeta, req *ordersv1.StopsBeforeForOrderRequest) (*ordersv1.StopsBeforeForOrderResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.StopsBeforeForOrder(ctx, req)
}

// AdvanceRouteStop mueve un pedido de la ruta del repartidor. El Gateway ya verificó que la ruta es suya.
func (o *Orders) AdvanceRouteStop(ctx context.Context, m CallMeta, req *ordersv1.AdvanceRouteStopRequest) (*ordersv1.AdvanceRouteStopResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.AdvanceRouteStop(ctx, req)
}

// RecordRouteStopOutcome cierra una parada de la ruta del repartidor como DELIVERED o NOT_DELIVERED.
// Solo se llama después de que Driver Service aceptó el registro de entrega.
func (o *Orders) RecordRouteStopOutcome(ctx context.Context, m CallMeta, req *ordersv1.RecordRouteStopOutcomeRequest) (*ordersv1.RecordRouteStopOutcomeResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.RecordRouteStopOutcome(ctx, req)
}

// ListDriverRouteStops devuelve las paradas de la ruta del conductor, en orden de secuencia.
func (o *Orders) ListDriverRouteStops(ctx context.Context, m CallMeta, req *ordersv1.ListDriverRouteStopsRequest) (*ordersv1.ListDriverRouteStopsResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.ListDriverRouteStops(ctx, req)
}

// ListOrders devuelve los pedidos de todos los clientes para el personal.
func (o *Orders) ListOrders(ctx context.Context, m CallMeta, req *ordersv1.ListOrdersRequest) (*ordersv1.ListOrdersResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.ListOrders(ctx, req)
}

// SetReassignmentFee define la tarifa fija de reasignación (solo ADMIN).
func (o *Orders) SetReassignmentFee(ctx context.Context, m CallMeta, req *ordersv1.SetReassignmentFeeRequest) (*ordersv1.SetReassignmentFeeResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.SetReassignmentFee(ctx, req)
}

// GetReassignmentFee devuelve la tarifa fija vigente al personal.
func (o *Orders) GetReassignmentFee(ctx context.Context, m CallMeta, req *ordersv1.GetReassignmentFeeRequest) (*ordersv1.GetReassignmentFeeResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.GetReassignmentFee(ctx, req)
}

// ApplyReassignmentCharge cobra la tarifa fija vigente por la reasignación de un pedido.
func (o *Orders) ApplyReassignmentCharge(ctx context.Context, m CallMeta, req *ordersv1.ApplyReassignmentChargeRequest) (*ordersv1.ApplyReassignmentChargeResponse, error) {
	ctx, cancel := o.outgoing(ctx, m)
	defer cancel()
	return o.client.ApplyReassignmentCharge(ctx, req)
}
