package grpc

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
)

// PayMyBalance pays the authenticated customer's whole balance with the simulated card. The customer is
// the verified identity; the request carries nothing, so no amount can be chosen.
func (s *OrderServer) PayMyBalance(ctx context.Context, _ *ordersv1.PayMyBalanceRequest) (*ordersv1.PayMyBalanceResponse, error) {
	if s.useCases.Pay == nil {
		return nil, status.Error(codes.Unimplemented, "pay balance is not wired")
	}
	userID, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	res, err := s.useCases.Pay.Run(ctx, orderapp.PayBalanceCommand{UserID: userID, Role: role})
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.PayMyBalanceResponse{
		Status:    string(res.Attempt.Status),
		Amount:    res.Attempt.Amount.StringFixed(2),
		Reference: res.Attempt.Reference,
		Unblocked: res.Unblocked,
		CreatedAt: res.Attempt.CreatedAt.UTC().Format(time.RFC3339),
	}, nil
}
