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
