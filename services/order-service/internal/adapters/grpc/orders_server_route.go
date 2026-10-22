package grpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
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
		Stops:    routeStopsFromProto(req.GetStops()),
	})
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.AssignOrdersToRouteResponse{Orders: ordersToProto(orders)}, nil
}

// routeStopsFromProto convierte las paradas con su secuencia. La validación (rango, unicidad) la hace
// el caso de uso, para que las reglas vivan en un solo lugar.
func routeStopsFromProto(stops []*ordersv1.OrderStop) []orderapp.RouteStop {
	out := make([]orderapp.RouteStop, 0, len(stops))
	for _, stop := range stops {
		out = append(out, orderapp.RouteStop{OrderID: stop.GetOrderId(), Sequence: int(stop.GetSequence())})
	}
	return out
}

// AdvanceRouteStop mueve un pedido de la ruta del conductor a PICKED_UP o IN_TRANSIT. Solo DRIVER.
// El Gateway verifica antes que el conductor es dueño de la ruta. Un pedido fuera de la ruta responde
// NOT_FOUND; un paso ilegal responde FAILED_PRECONDITION.
func (s *OrderServer) AdvanceRouteStop(ctx context.Context, req *ordersv1.AdvanceRouteStopRequest) (*ordersv1.AdvanceRouteStopResponse, error) {
	if s.useCases.AdvanceStop == nil {
		return nil, status.Error(codes.Unimplemented, "advance route stop is not wired")
	}
	_, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	order, err := s.useCases.AdvanceStop.Run(ctx, orderapp.AdvanceRouteStopCommand{
		Role:    role,
		RouteID: req.GetRouteId(),
		OrderID: req.GetOrderId(),
		Target:  routeStopTargetFromProto(req.GetTargetStatus()),
	})
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.AdvanceRouteStopResponse{Order: orderToProto(order)}, nil
}

// StopsBeforeForOrder devuelve cuántas paradas abiertas quedan antes del pedido del cliente autenticado.
// Solo CUSTOMER, y solo sobre un pedido propio: uno ajeno o inexistente responde NOT_FOUND.
func (s *OrderServer) StopsBeforeForOrder(ctx context.Context, req *ordersv1.StopsBeforeForOrderRequest) (*ordersv1.StopsBeforeForOrderResponse, error) {
	if s.useCases.StopsBefore == nil {
		return nil, status.Error(codes.Unimplemented, "stops before is not wired")
	}
	userID, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	count, err := s.useCases.StopsBefore.Run(ctx, orderapp.StopsBeforeCommand{
		UserID:  userID,
		Role:    role,
		OrderID: req.GetOrderId(),
	})
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.StopsBeforeForOrderResponse{StopsBefore: int32(count)}, nil
}

// ListDriverRouteStops devuelve las paradas de la ruta del conductor, en orden de secuencia.
// Solo DRIVER (otro rol responde PERMISSION_DENIED sin leer). El Gateway verifica antes que la ruta es del
// conductor. La respuesta no incluye datos del cliente: ni nombre, ni teléfono, ni su id.
func (s *OrderServer) ListDriverRouteStops(ctx context.Context, req *ordersv1.ListDriverRouteStopsRequest) (*ordersv1.ListDriverRouteStopsResponse, error) {
	if s.useCases.DriverRouteStops == nil {
		return nil, status.Error(codes.Unimplemented, "driver route stops is not wired")
	}
	_, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	orders, err := s.useCases.DriverRouteStops.Run(ctx, orderapp.DriverRouteStopsCommand{Role: role, RouteID: req.GetRouteId()})
	if err != nil {
		return nil, mapOrderError(err)
	}
	stops := make([]*ordersv1.DriverRouteStop, 0, len(orders))
	for _, o := range orders {
		stops = append(stops, &ordersv1.DriverRouteStop{
			OrderId:     o.ID,
			Sequence:    int32(o.StopSequence),
			Address:     o.Address,
			Latitude:    o.Latitude,
			Longitude:   o.Longitude,
			WindowStart: o.Window.Start(),
			WindowEnd:   o.Window.End(),
			Status:      orderToProto(o).GetStatus(),
		})
	}
	return &ordersv1.ListDriverRouteStopsResponse{Stops: stops}, nil
}

// RecordRouteStopOutcome cierra una parada de la ruta del conductor como DELIVERED o NOT_DELIVERED. Solo
// DRIVER. El Gateway llama aqui solo despues de que Driver Service acepto el registro de entrega.
// Un pedido fuera de la ruta responde NOT_FOUND; un pedido que no esta IN_TRANSIT responde FAILED_PRECONDITION.
func (s *OrderServer) RecordRouteStopOutcome(ctx context.Context, req *ordersv1.RecordRouteStopOutcomeRequest) (*ordersv1.RecordRouteStopOutcomeResponse, error) {
	if s.useCases.RecordOutcome == nil {
		return nil, status.Error(codes.Unimplemented, "record route stop outcome is not wired")
	}
	_, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	order, err := s.useCases.RecordOutcome.Run(ctx, orderapp.RecordStopOutcomeCommand{
		Role:    role,
		RouteID: req.GetRouteId(),
		OrderID: req.GetOrderId(),
		Outcome: routeStopOutcomeFromProto(req.GetOutcome()),
	})
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.RecordRouteStopOutcomeResponse{Order: orderToProto(order)}, nil
}

// routeStopOutcomeFromProto traduce el resultado pedido. Cualquier valor distinto de DELIVERED o
// NOT_DELIVERED llega al dominio como estado vacio y se rechaza como INVALID_ARGUMENT.
func routeStopOutcomeFromProto(outcome ordersv1.OrderStatus) domain.Status {
	switch outcome {
	case ordersv1.OrderStatus_ORDER_STATUS_DELIVERED:
		return domain.StatusDelivered
	case ordersv1.OrderStatus_ORDER_STATUS_NOT_DELIVERED:
		return domain.StatusNotDelivered
	default:
		return ""
	}
}

// routeStopTargetFromProto traduce el estado pedido. Cualquier valor que no sea PICKED_UP o IN_TRANSIT
// llega al dominio como estado vacío y se rechaza como INVALID_ARGUMENT.
func routeStopTargetFromProto(target ordersv1.OrderStatus) domain.Status {
	switch target {
	case ordersv1.OrderStatus_ORDER_STATUS_PICKED_UP:
		return domain.StatusPickedUp
	case ordersv1.OrderStatus_ORDER_STATUS_IN_TRANSIT:
		return domain.StatusInTransit
	default:
		return ""
	}
}
