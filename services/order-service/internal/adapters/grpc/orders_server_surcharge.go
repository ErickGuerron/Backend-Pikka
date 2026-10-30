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

// ListSurchargesPendingVerification returns the operator's queue (ADMIN and OPERATOR).
func (s *OrderServer) ListSurchargesPendingVerification(ctx context.Context, _ *ordersv1.ListSurchargesPendingVerificationRequest) (*ordersv1.ListSurchargesPendingVerificationResponse, error) {
	if s.useCases.Surcharges == nil {
		return nil, status.Error(codes.Unimplemented, "surcharge verification is not wired")
	}
	_, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	list, err := s.useCases.Surcharges.ListPending(ctx, role)
	if err != nil {
		return nil, mapOrderError(err)
	}
	out := make([]*ordersv1.Surcharge, 0, len(list))
	for _, item := range list {
		out = append(out, surchargeToProto(item))
	}
	return &ordersv1.ListSurchargesPendingVerificationResponse{Surcharges: out}, nil
}

// ResolveSurchargeVerification confirms or waives one surcharge. The operator's identity is stored
// as audit, so it comes from the verified metadata and never from the request body.
func (s *OrderServer) ResolveSurchargeVerification(ctx context.Context, req *ordersv1.ResolveSurchargeVerificationRequest) (*ordersv1.ResolveSurchargeVerificationResponse, error) {
	if s.useCases.Surcharges == nil {
		return nil, status.Error(codes.Unimplemented, "surcharge verification is not wired")
	}
	userID, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	resolved, err := s.useCases.Surcharges.Run(ctx, orderapp.ResolveSurchargeCommand{
		Role:        role,
		OperatorID:  userID.String(),
		SurchargeID: req.GetId(),
		Decision:    req.GetDecision(),
		Reason:      req.GetReason(),
	})
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.ResolveSurchargeVerificationResponse{Surcharge: surchargeToProto(resolved)}, nil
}

func surchargeToProto(s domain.Surcharge) *ordersv1.Surcharge {
	out := &ordersv1.Surcharge{
		Id:               s.ID,
		CustomerId:       s.CustomerID,
		OrderId:          s.OrderID,
		Reason:           s.Reason,
		Status:           string(s.Status),
		CreatedAt:        s.CreatedAt.UTC().Format(time.RFC3339),
		ResolvedBy:       s.ResolvedBy,
		ResolutionReason: s.ResolutionReason,
	}
	if s.Amount.Valid {
		out.Amount = s.Amount.Decimal.StringFixed(2)
	}
	if !s.ResolvedAt.IsZero() {
		out.ResolvedAt = s.ResolvedAt.UTC().Format(time.RFC3339)
	}
	return out
}
