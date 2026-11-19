package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

type reassignmentStore struct {
	ports.ReassignmentRepository
	fee   *domain.ReassignmentFee
	err   error
	calls int
}

func (s *reassignmentStore) SetReassignmentFee(_ context.Context, fee domain.ReassignmentFee) (domain.ReassignmentFee, error) {
	s.calls++
	fee.ID = uuid.NewString()
	return fee, s.err
}

func (s *reassignmentStore) CurrentReassignmentFee(context.Context) (domain.ReassignmentFee, error) {
	s.calls++
	if s.err != nil {
		return domain.ReassignmentFee{}, s.err
	}
	if s.fee == nil {
		return domain.ReassignmentFee{}, domain.ErrReassignmentFeeNotSet
	}
	return *s.fee, nil
}

func (s *reassignmentStore) ApplyReassignmentCharge(_ context.Context, _ string, build func(string, *domain.ReassignmentFee) (domain.ReassignmentCharge, error)) (domain.ReassignmentCharge, error) {
	s.calls++
	if s.err != nil {
		return domain.ReassignmentCharge{}, s.err
	}
	c, err := build(uuid.NewString(), s.fee)
	c.ID = uuid.NewString()
	return c, err
}

func reassignmentServer(store *reassignmentStore) *OrderServer {
	return NewOrderServer(OrderUseCases{
		SetReassignmentFee:      orderapp.NewSetReassignmentFee(store),
		GetReassignmentFee:      orderapp.NewGetReassignmentFee(store),
		ApplyReassignmentCharge: orderapp.NewApplyReassignmentCharge(store),
	})
}

func fee250() *domain.ReassignmentFee {
	return &domain.ReassignmentFee{ID: "f-1", Amount: decimal.RequireFromString("2.50"), Reason: "Sube la gasolina", SetBy: "a-1", CreatedAt: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)}
}

func TestSetReassignmentFeeReturnsTheAuditedFee(t *testing.T) {
	res, err := reassignmentServer(&reassignmentStore{}).SetReassignmentFee(asRole("ADMIN"), &ordersv1.SetReassignmentFeeRequest{Amount: "2.5", Reason: "Sube la gasolina"})
	if err != nil {
		t.Fatal(err)
	}
	got := res.GetFee()
	if got.GetAmount() != "2.50" || got.GetSetBy() == "" || got.GetId() == "" || got.GetReason() != "Sube la gasolina" || got.GetCreatedAt() == "" {
		t.Fatalf("fee = %+v", got)
	}
}

func TestSetReassignmentFeeIsAdminOnlyAndValidated(t *testing.T) {
	cases := []struct {
		name, role, amount string
		want               codes.Code
	}{
		{"operator", "OPERATOR", "2.50", codes.PermissionDenied},
		{"zero", "ADMIN", "0", codes.InvalidArgument},
		{"not a number", "ADMIN", "abc", codes.InvalidArgument},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &reassignmentStore{}
			_, err := reassignmentServer(store).SetReassignmentFee(asRole(tc.role), &ordersv1.SetReassignmentFeeRequest{Amount: tc.amount, Reason: "Sube la gasolina"})
			if status.Code(err) != tc.want {
				t.Fatalf("code = %v (%v), want %v", status.Code(err), err, tc.want)
			}
			if store.calls != 0 {
				t.Fatalf("repository called %d times", store.calls)
			}
		})
	}
}

func TestGetReassignmentFee(t *testing.T) {
	res, err := reassignmentServer(&reassignmentStore{fee: fee250()}).GetReassignmentFee(asRole("OPERATOR"), &ordersv1.GetReassignmentFeeRequest{})
	if err != nil || res.GetFee().GetAmount() != "2.50" {
		t.Fatalf("res = %v err = %v", res, err)
	}
	if _, err := reassignmentServer(&reassignmentStore{}).GetReassignmentFee(asRole("ADMIN"), &ordersv1.GetReassignmentFeeRequest{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("unset code = %v", status.Code(err))
	}
	if _, err := reassignmentServer(&reassignmentStore{fee: fee250()}).GetReassignmentFee(asRole("CUSTOMER"), &ordersv1.GetReassignmentFeeRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("customer code = %v", status.Code(err))
	}
}

func reassignmentReq() *ordersv1.ApplyReassignmentChargeRequest {
	return &ordersv1.ApplyReassignmentChargeRequest{OrderId: uuid.NewString(), Reason: "Pidió cambiar la entrega con el repartidor en camino"}
}

func TestApplyReassignmentChargeReturnsTheAuditedCharge(t *testing.T) {
	res, err := reassignmentServer(&reassignmentStore{fee: fee250()}).ApplyReassignmentCharge(asRole("OPERATOR"), reassignmentReq())
	if err != nil {
		t.Fatal(err)
	}
	got := res.GetCharge()
	if got.GetAmount() != "2.50" || got.GetAppliedBy() == "" || got.GetOrderId() == "" || got.GetCustomerId() == "" || got.GetId() == "" || got.GetCreatedAt() == "" {
		t.Fatalf("charge = %+v", got)
	}
}

func TestApplyReassignmentChargeErrorCodes(t *testing.T) {
	cases := []struct {
		name string
		role string
		err  error
		fee  *domain.ReassignmentFee
		want codes.Code
	}{
		{"customer", "CUSTOMER", nil, fee250(), codes.PermissionDenied},
		{"fee not set", "OPERATOR", nil, nil, codes.FailedPrecondition},
		{"already charged", "OPERATOR", domain.ErrReassignmentChargeExists, fee250(), codes.Aborted},
		{"unknown order", "ADMIN", domain.ErrOrderNotFound, fee250(), codes.NotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := reassignmentServer(&reassignmentStore{fee: tc.fee, err: tc.err}).ApplyReassignmentCharge(asRole(tc.role), reassignmentReq())
			if status.Code(err) != tc.want {
				t.Fatalf("code = %v (%v), want %v", status.Code(err), err, tc.want)
			}
		})
	}
}

func TestReassignmentIsUnimplementedWhenNotWired(t *testing.T) {
	srv := NewOrderServer(OrderUseCases{})
	if _, err := srv.SetReassignmentFee(asRole("ADMIN"), &ordersv1.SetReassignmentFeeRequest{}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("set code = %v", status.Code(err))
	}
	if _, err := srv.GetReassignmentFee(asRole("ADMIN"), &ordersv1.GetReassignmentFeeRequest{}); status.Code(err) != codes.Unimplemented {
		t.Fatalf("get code = %v", status.Code(err))
	}
	if _, err := srv.ApplyReassignmentCharge(asRole("ADMIN"), reassignmentReq()); status.Code(err) != codes.Unimplemented {
		t.Fatalf("apply code = %v", status.Code(err))
	}
}
