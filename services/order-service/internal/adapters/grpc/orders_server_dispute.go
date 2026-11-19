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

// OpenDispute lets the authenticated customer dispute one of their own surcharges. The customer comes from the verified
// identity, never from the request.
func (s *OrderServer) OpenDispute(ctx context.Context, req *ordersv1.OpenDisputeRequest) (*ordersv1.OpenDisputeResponse, error) {
	if s.useCases.OpenDispute == nil {
		return nil, status.Error(codes.Unimplemented, "disputes are not wired")
	}
	userID, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	d, err := s.useCases.OpenDispute.Run(ctx, orderapp.OpenDisputeCommand{
		UserID: userID, Role: role, SurchargeID: req.GetSurchargeId(), Reason: req.GetReason(),
	})
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.OpenDisputeResponse{Dispute: disputeToProto(d)}, nil
}

// ListOpenDisputes returns the staff's queue (ADMIN and OPERATOR).
func (s *OrderServer) ListOpenDisputes(ctx context.Context, _ *ordersv1.ListOpenDisputesRequest) (*ordersv1.ListOpenDisputesResponse, error) {
	if s.useCases.ListOpenDisputes == nil {
		return nil, status.Error(codes.Unimplemented, "disputes are not wired")
	}
	_, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	list, err := s.useCases.ListOpenDisputes.Run(ctx, role)
	if err != nil {
		return nil, mapOrderError(err)
	}
	out := make([]*ordersv1.Dispute, 0, len(list))
	for _, d := range list {
		out = append(out, disputeToProto(d))
	}
	return &ordersv1.ListOpenDisputesResponse{Disputes: out}, nil
}

// ResolveDispute decides one dispute. The deciding operator is stored as audit, so it comes from the verified metadata.
func (s *OrderServer) ResolveDispute(ctx context.Context, req *ordersv1.ResolveDisputeRequest) (*ordersv1.ResolveDisputeResponse, error) {
	if s.useCases.ResolveDispute == nil {
		return nil, status.Error(codes.Unimplemented, "disputes are not wired")
	}
	userID, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	d, err := s.useCases.ResolveDispute.Run(ctx, orderapp.ResolveDisputeCommand{
		Role: role, OperatorID: userID.String(), DisputeID: req.GetId(), Decision: req.GetDecision(), Reason: req.GetReason(),
	})
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.ResolveDisputeResponse{Dispute: disputeToProto(d)}, nil
}

func disputeToProto(d domain.Dispute) *ordersv1.Dispute {
	out := &ordersv1.Dispute{
		Id: d.ID, SurchargeId: d.SurchargeID, CustomerId: d.CustomerID, Reason: d.Reason,
		Status: string(d.Status), OpenedAt: d.OpenedAt.UTC().Format(time.RFC3339),
		ResolvedBy: d.ResolvedBy, ResolutionReason: d.ResolutionReason,
	}
	if !d.ResolvedAt.IsZero() {
		out.ResolvedAt = d.ResolvedAt.UTC().Format(time.RFC3339)
	}
	return out
}
