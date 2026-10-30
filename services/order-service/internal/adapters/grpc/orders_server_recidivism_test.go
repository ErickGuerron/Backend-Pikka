package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

type recidivismStore struct {
	ports.RecidivismRepository
	err    error
	called int
}

func (s *recidivismStore) ApplyRecidivismCharge(_ context.Context, customerID string, build func(time.Time) (domain.RecidivismCharge, error)) (domain.RecidivismCharge, error) {
	s.called++
	if s.err != nil {
		return domain.RecidivismCharge{}, s.err
	}
	charge, err := build(time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC))
	charge.ID = uuid.NewString()
	return charge, err
}

func recidivismServer(store *recidivismStore) *OrderServer {
	return NewOrderServer(OrderUseCases{Recidivism: orderapp.NewApplyRecidivismCharge(store)})
}

func recidivismReq() *ordersv1.ApplyRecidivismChargeRequest {
	return &ordersv1.ApplyRecidivismChargeRequest{CustomerId: uuid.NewString(), Amount: "12.50", Reason: "Tres no entregas en dos semanas"}
}

func TestApplyRecidivismChargeReturnsTheAuditedCharge(t *testing.T) {
	res, err := recidivismServer(&recidivismStore{}).ApplyRecidivismCharge(asRole("OPERATOR"), recidivismReq())
	if err != nil {
		t.Fatal(err)
	}
	got := res.GetCharge()
	if got.GetAmount() != "12.50" || got.GetReason() != "Tres no entregas en dos semanas" || got.GetAppliedBy() == "" ||
		got.GetRecidivistAt() != "2026-10-10T09:00:00Z" || got.GetId() == "" || got.GetCreatedAt() == "" {
		t.Fatalf("charge = %+v", got)
	}
}

func TestApplyRecidivismChargeErrorCodes(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
		req  func() *ordersv1.ApplyRecidivismChargeRequest
		err  error
		want codes.Code
	}{
		{"customer role", asRole("CUSTOMER"), recidivismReq, nil, codes.PermissionDenied},
		{"driver role", asRole("DRIVER"), recidivismReq, nil, codes.PermissionDenied},
		{"no identity", context.Background(), recidivismReq, nil, codes.Unauthenticated},
		{"not recidivist", asRole("OPERATOR"), recidivismReq, domain.ErrCustomerNotRecidivist, codes.FailedPrecondition},
		{"already charged", asRole("OPERATOR"), recidivismReq, domain.ErrRecidivismChargeExists, codes.Aborted},
		{"unknown customer", asRole("OPERATOR"), recidivismReq, domain.ErrCustomerNotFound, codes.NotFound},
		{"zero amount", asRole("OPERATOR"), func() *ordersv1.ApplyRecidivismChargeRequest {
			r := recidivismReq()
			r.Amount = "0"
			return r
		}, nil, codes.InvalidArgument},
		{"amount not a number", asRole("OPERATOR"), func() *ordersv1.ApplyRecidivismChargeRequest {
			r := recidivismReq()
			r.Amount = "doce"
			return r
		}, nil, codes.InvalidArgument},
		{"short reason", asRole("OPERATOR"), func() *ordersv1.ApplyRecidivismChargeRequest {
			r := recidivismReq()
			r.Reason = "no"
			return r
		}, nil, codes.InvalidArgument},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := recidivismServer(&recidivismStore{err: tc.err}).ApplyRecidivismCharge(tc.ctx, tc.req())
			wantCode(t, err, tc.want)
		})
	}
}

func TestApplyRecidivismChargeInvalidInputNeverReachesTheRepository(t *testing.T) {
	store := &recidivismStore{}
	req := recidivismReq()
	req.Amount = "-3"

	_, err := recidivismServer(store).ApplyRecidivismCharge(asRole("OPERATOR"), req)

	wantCode(t, err, codes.InvalidArgument)
	if store.called != 0 {
		t.Fatalf("repository called %d times", store.called)
	}
}

func TestApplyRecidivismChargeNotWired(t *testing.T) {
	_, err := NewOrderServer(OrderUseCases{}).ApplyRecidivismCharge(asRole("OPERATOR"), recidivismReq())
	wantCode(t, err, codes.Unimplemented)
}
