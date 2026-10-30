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

// ApplyRecidivismCharge applies the operator's extra charge to a recidivist customer. The operator
// stored as audit comes from the verified metadata, never from the request.
func (s *OrderServer) ApplyRecidivismCharge(ctx context.Context, req *ordersv1.ApplyRecidivismChargeRequest) (*ordersv1.ApplyRecidivismChargeResponse, error) {
	if s.useCases.Recidivism == nil {
		return nil, status.Error(codes.Unimplemented, "recidivism charge is not wired")
	}
	userID, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	charge, err := s.useCases.Recidivism.Run(ctx, orderapp.ApplyRecidivismChargeCommand{
		Role:       role,
		OperatorID: userID.String(),
		CustomerID: req.GetCustomerId(),
		Amount:     req.GetAmount(),
		Reason:     req.GetReason(),
	})
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.ApplyRecidivismChargeResponse{Charge: recidivismChargeToProto(charge)}, nil
}

func recidivismChargeToProto(c domain.RecidivismCharge) *ordersv1.RecidivismCharge {
	return &ordersv1.RecidivismCharge{
		Id:           c.ID,
		CustomerId:   c.CustomerID,
		RecidivistAt: c.RecidivistAt.UTC().Format(time.RFC3339),
		Amount:       c.Amount.StringFixed(2),
		Reason:       c.Reason,
		AppliedBy:    c.AppliedBy,
		CreatedAt:    c.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// ListRecidivists lists the customers with the reincidence mark for the operator. The role comes from the
// verified metadata.
func (s *OrderServer) ListRecidivists(ctx context.Context, _ *ordersv1.ListRecidivistsRequest) (*ordersv1.ListRecidivistsResponse, error) {
	if s.useCases.ListRecidivists == nil {
		return nil, status.Error(codes.Unimplemented, "recidivist list is not wired")
	}
	_, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	list, err := s.useCases.ListRecidivists.Run(ctx, role)
	if err != nil {
		return nil, mapOrderError(err)
	}
	out := make([]*ordersv1.Recidivist, 0, len(list))
	for _, r := range list {
		out = append(out, &ordersv1.Recidivist{
			CustomerId:       r.CustomerID,
			FullName:         r.FullName,
			ContactPhone:     r.ContactPhone,
			RecidivistAt:     r.RecidivistAt.UTC().Format(time.RFC3339),
			PendingSurcharge: r.PendingSurcharge.StringFixed(2),
			ChargeApplied:    r.ChargeApplied,
		})
	}
	return &ordersv1.ListRecidivistsResponse{Recidivists: out}, nil
}
