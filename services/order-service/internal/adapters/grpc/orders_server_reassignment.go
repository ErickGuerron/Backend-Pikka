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

// SetReassignmentFee defines the fixed reassignment fee. The administrator stored as audit comes from the
// verified metadata, never from the request.
func (s *OrderServer) SetReassignmentFee(ctx context.Context, req *ordersv1.SetReassignmentFeeRequest) (*ordersv1.SetReassignmentFeeResponse, error) {
	if s.useCases.SetReassignmentFee == nil {
		return nil, status.Error(codes.Unimplemented, "reassignment fee is not wired")
	}
	userID, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	fee, err := s.useCases.SetReassignmentFee.Run(ctx, orderapp.SetReassignmentFeeCommand{
		Role: role, AdminID: userID.String(), Amount: req.GetAmount(), Reason: req.GetReason(),
	})
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.SetReassignmentFeeResponse{Fee: reassignmentFeeToProto(fee)}, nil
}

// GetReassignmentFee returns the current fixed fee to the staff.
func (s *OrderServer) GetReassignmentFee(ctx context.Context, _ *ordersv1.GetReassignmentFeeRequest) (*ordersv1.GetReassignmentFeeResponse, error) {
	if s.useCases.GetReassignmentFee == nil {
		return nil, status.Error(codes.Unimplemented, "reassignment fee is not wired")
	}
	_, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	fee, err := s.useCases.GetReassignmentFee.Run(ctx, role)
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.GetReassignmentFeeResponse{Fee: reassignmentFeeToProto(fee)}, nil
}

// ApplyReassignmentCharge charges the current fee to the customer of the order. The amount is never read from
// the request and the operator comes from the verified metadata.
func (s *OrderServer) ApplyReassignmentCharge(ctx context.Context, req *ordersv1.ApplyReassignmentChargeRequest) (*ordersv1.ApplyReassignmentChargeResponse, error) {
	if s.useCases.ApplyReassignmentCharge == nil {
		return nil, status.Error(codes.Unimplemented, "reassignment charge is not wired")
	}
	userID, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	charge, err := s.useCases.ApplyReassignmentCharge.Run(ctx, orderapp.ApplyReassignmentChargeCommand{
		Role: role, OperatorID: userID.String(), OrderID: req.GetOrderId(), Reason: req.GetReason(),
	})
	if err != nil {
		return nil, mapOrderError(err)
	}
	return &ordersv1.ApplyReassignmentChargeResponse{Charge: &ordersv1.ReassignmentCharge{
		Id: charge.ID, CustomerId: charge.CustomerID, OrderId: charge.OrderID, Amount: charge.Amount.StringFixed(2),
		Reason: charge.Reason, AppliedBy: charge.AppliedBy, CreatedAt: charge.CreatedAt.UTC().Format(time.RFC3339),
	}}, nil
}

func reassignmentFeeToProto(f domain.ReassignmentFee) *ordersv1.ReassignmentFee {
	return &ordersv1.ReassignmentFee{
		Id: f.ID, Amount: f.Amount.StringFixed(2), Reason: f.Reason, SetBy: f.SetBy, CreatedAt: f.CreatedAt.UTC().Format(time.RFC3339),
	}
}
