package grpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
)

// GetPendingOrders devuelve los pedidos PENDING de una fecha de entrega para la planificación de Routing.
// Solo ADMIN y OPERATOR: el rol llega en la identidad propagada por el Gateway.
func (s *OrderServer) GetPendingOrders(ctx context.Context, req *ordersv1.GetPendingOrdersRequest) (*ordersv1.GetPendingOrdersResponse, error) {
	if s.useCases.RouteAssignment == nil {
		return nil, status.Error(codes.Unimplemented, "route assignment is not wired")
	}
	_, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	orders, err := s.useCases.RouteAssignment.PendingByDate(ctx, role, req.GetDeliveryDate())
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.GetPendingOrdersResponse{Orders: ordersToProto(orders)}, nil
}

// AssignOrdersToRoute mueve todos los pedidos indicados a ROUTE_ASSIGNED, o ninguno.
// Solo ADMIN y OPERATOR.
func (s *OrderServer) AssignOrdersToRoute(ctx context.Context, req *ordersv1.AssignOrdersToRouteRequest) (*ordersv1.AssignOrdersToRouteResponse, error) {
	if s.useCases.RouteAssignment == nil {
		return nil, status.Error(codes.Unimplemented, "route assignment is not wired")
	}
	_, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	orders, err := s.useCases.RouteAssignment.Assign(ctx, orderapp.AssignToRouteCommand{
		Role:     role,
		RouteID:  req.GetRouteId(),
		OrderIDs: req.GetOrderIds(),
	})
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.AssignOrdersToRouteResponse{Orders: ordersToProto(orders)}, nil
}
