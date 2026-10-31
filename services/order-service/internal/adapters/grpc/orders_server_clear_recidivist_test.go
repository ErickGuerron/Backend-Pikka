package grpc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

func (s *recidivismStore) ClearRecidivistMark(_ context.Context, _ string, build func(time.Time) (domain.RecidivistClear, error)) (domain.RecidivistClear, error) {
	s.called++
	if s.err != nil {
		return domain.RecidivistClear{}, s.err
	}
	c, err := build(time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC))
	c.ID = uuid.NewString()
	return c, err
}

func clearServer(store *recidivismStore) *OrderServer {
	return NewOrderServer(OrderUseCases{ClearRecidivist: orderapp.NewClearRecidivistMark(store)})
}

func clearReq() *ordersv1.ClearRecidivistRequest {
	return &ordersv1.ClearRecidivistRequest{CustomerId: uuid.NewString(), Reason: "Pagó todo y se acordó con él"}
}

func TestClearRecidivistReturnsTheAuditRow(t *testing.T) {
	res, err := clearServer(&recidivismStore{}).ClearRecidivist(asRole("ADMIN"), clearReq())
	if err != nil {
		t.Fatal(err)
	}
	got := res.GetClear()
	if got.GetId() == "" || got.GetClearedBy() == "" || got.GetRecidivistAt() != "2026-10-10T09:00:00Z" || got.GetReason() == "" || got.GetCreatedAt() == "" {
		t.Fatalf("clear = %+v", got)
	}
}

func TestClearRecidivistErrorCodes(t *testing.T) {
	cases := []struct {
		name string
		role string
		err  error
		want codes.Code
	}{
		{"operator is denied", "OPERATOR", nil, codes.PermissionDenied},
		{"not marked", "ADMIN", domain.ErrCustomerNotRecidivist, codes.FailedPrecondition},
		{"unknown customer", "ADMIN", domain.ErrCustomerNotFound, codes.NotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := clearServer(&recidivismStore{err: tc.err}).ClearRecidivist(asRole(tc.role), clearReq())
			if status.Code(err) != tc.want {
				t.Fatalf("code = %v (%v), want %v", status.Code(err), err, tc.want)
			}
		})
	}
}

func TestClearRecidivistIsUnimplementedWhenNotWired(t *testing.T) {
	_, err := NewOrderServer(OrderUseCases{}).ClearRecidivist(asRole("ADMIN"), clearReq())
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("code = %v", status.Code(err))
	}
}

func TestApplyRecidivismChargeAboveTheCapIsInvalidAndNamesTheCap(t *testing.T) {
	store := &recidivismStore{}
	srv := NewOrderServer(OrderUseCases{Recidivism: orderapp.NewApplyRecidivismCharge(store, decimal.NewFromInt(50))})
	req := recidivismReq()
	req.Amount = "1250.00"

	_, err := srv.ApplyRecidivismCharge(asRole("OPERATOR"), req)

	if status.Code(err) != codes.InvalidArgument || !strings.Contains(err.Error(), "50.00") {
		t.Fatalf("err = %v", err)
	}
	if store.called != 0 {
		t.Fatalf("repository called %d times", store.called)
	}
}
