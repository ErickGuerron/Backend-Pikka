package grpc

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// GetMyBalance shows the authenticated customer what they owe. A blocked account can call it: that is how
// it learns what to pay.
func (s *OrderServer) GetMyBalance(ctx context.Context, _ *ordersv1.GetMyBalanceRequest) (*ordersv1.GetMyBalanceResponse, error) {
	if s.useCases.Balance == nil {
		return nil, status.Error(codes.Unimplemented, "my balance is not wired")
	}
	userID, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	balance, err := s.useCases.Balance.Run(ctx, orderapp.GetBalanceCommand{UserID: userID, Role: role})
	if err != nil {
		return nil, mapOrderError(err)
	}
	lines := make([]*ordersv1.BalanceLine, 0, len(balance.Lines))
	for _, l := range balance.Lines {
		lines = append(lines, &ordersv1.BalanceLine{
			Kind:            string(l.Kind),
			Id:              l.ID,
			OrderId:         l.OrderID,
			Reason:          l.Reason,
			Amount:          l.Amount.StringFixed(2),
			StartedAt:       l.StartedAt.UTC().Format(time.RFC3339),
			DueAt:           l.DueAt().UTC().Format(time.RFC3339),
			DisputeStatus:   l.DisputeStatus,
			DisputableUntil: disputableUntil(l),
		})
	}
	return &ordersv1.GetMyBalanceResponse{
		PendingSurcharge: balance.PendingSurcharge.StringFixed(2),
		AccountStatus:    string(balance.Status),
		Lines:            lines,
	}, nil
}

// disputableUntil is empty when the customer can no longer, or never could, dispute the line.
func disputableUntil(l domain.BalanceLine) string {
	if until := l.DisputableUntil(); !until.IsZero() {
		return until.UTC().Format(time.RFC3339)
	}
	return ""
}
