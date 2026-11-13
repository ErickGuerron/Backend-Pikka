package grpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

func (s *OrderServer) EditMyOrder(ctx context.Context, req *ordersv1.EditMyOrderRequest) (*ordersv1.EditMyOrderResponse, error) {
	if s.useCases.EditMine == nil {
		return nil, status.Error(codes.Unimplemented, "edit order is not wired")
	}
	userID, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	order, err := s.useCases.EditMine.Run(ctx, userID, role, req.GetOrderId(), domain.CustomerOrderEdit{
		Address: req.Address, Latitude: req.Latitude, Longitude: req.Longitude,
		DeliveryDate: req.DeliveryDate, WindowStart: req.WindowStart, WindowEnd: req.WindowEnd,
		AddressReference: req.AddressReference, PostalCode: req.PostalCode, ContactPhone: req.ContactPhone,
	})
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.EditMyOrderResponse{Order: orderToProto(order)}, nil
}
