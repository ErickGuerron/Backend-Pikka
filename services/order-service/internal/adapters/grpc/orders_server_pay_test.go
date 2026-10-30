package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

type payCustomers struct {
	ports.CustomerRepository
	customer domain.Customer
}

func (p payCustomers) GetByUserID(context.Context, uuid.UUID) (domain.Customer, error) {
	return p.customer, nil
}

type payGateway struct{ status domain.PaymentStatus }

func (g payGateway) Charge(_ context.Context, r ports.ChargeRequest) (domain.PaymentAttempt, error) {
	return domain.NewPaymentAttempt(r.CustomerID, r.Amount, r.Method, g.status, "SIM-"+uuid.NewString(), time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC))
}

func (g payGateway) Refund(_ context.Context, r ports.RefundRequest) (domain.PaymentAttempt, error) {
	return domain.NewPaymentAttempt(r.CustomerID, r.Amount, r.Method, domain.PaymentApproved, "SIM-REFUND-"+uuid.NewString(), time.Now())
}

type payAttempts struct{ ports.PaymentAttemptRepository }

func (payAttempts) SavePaymentAttempt(_ context.Context, a domain.PaymentAttempt) (domain.PaymentAttempt, error) {
	a.ID = uuid.NewString()
	return a, nil
}

type payBalances struct {
	ports.BalanceRepository
	unblocked bool
	err       error
}

func (b payBalances) SettleBalance(context.Context, string, decimal.Decimal, time.Time) (bool, error) {
	return b.unblocked, b.err
}

func payServer(owed string, status domain.PaymentStatus, balances payBalances) *OrderServer {
	customer := domain.Customer{ID: uuid.New(), UserID: uuid.New(), Status: domain.StatusBlockedSurcharge, PendingSurcharge: decimal.RequireFromString(owed)}
	return NewOrderServer(OrderUseCases{Pay: orderapp.NewPayBalance(payCustomers{customer: customer}, payGateway{status}, payAttempts{}, balances)})
}

func customerCtx() context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(metadataUserID, uuid.NewString(), metadataUserRole, "CUSTOMER"))
}

func TestPayMyBalanceApprovedReportsTheUnblock(t *testing.T) {
	res, err := payServer("7.50", domain.PaymentApproved, payBalances{unblocked: true}).PayMyBalance(customerCtx(), &ordersv1.PayMyBalanceRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetStatus() != "SIMULATED_APPROVED" || res.GetAmount() != "7.50" || !res.GetUnblocked() || res.GetReference() == "" || res.GetCreatedAt() != "2026-10-29T09:00:00Z" {
		t.Fatalf("response = %+v", res)
	}
}

// A declined simulated card is an answer, not a gRPC error, and the account is not unblocked.
func TestPayMyBalanceDeclinedIsAnAnswer(t *testing.T) {
	res, err := payServer("7.99", domain.PaymentDeclined, payBalances{unblocked: true}).PayMyBalance(customerCtx(), &ordersv1.PayMyBalanceRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if res.GetStatus() != "SIMULATED_DECLINED" || res.GetUnblocked() {
		t.Fatalf("response = %+v", res)
	}
}

func TestPayMyBalanceErrorCodes(t *testing.T) {
	cases := []struct {
		name string
		srv  *OrderServer
		ctx  context.Context
		want codes.Code
	}{
		{"operator", payServer("7.50", domain.PaymentApproved, payBalances{}), asRole("OPERATOR"), codes.PermissionDenied},
		{"driver", payServer("7.50", domain.PaymentApproved, payBalances{}), asRole("DRIVER"), codes.PermissionDenied},
		{"no identity", payServer("7.50", domain.PaymentApproved, payBalances{}), context.Background(), codes.Unauthenticated},
		{"nothing owed", payServer("0", domain.PaymentApproved, payBalances{}), customerCtx(), codes.FailedPrecondition},
		{"balance changed", payServer("7.50", domain.PaymentApproved, payBalances{err: domain.ErrBalanceChanged}), customerCtx(), codes.Aborted},
		{"not wired", NewOrderServer(OrderUseCases{}), customerCtx(), codes.Unimplemented},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.srv.PayMyBalance(tc.ctx, &ordersv1.PayMyBalanceRequest{})
			wantCode(t, err, tc.want)
		})
	}
}
