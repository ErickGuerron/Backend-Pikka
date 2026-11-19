package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc/codes"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

type balanceLines struct {
	ports.BalanceRepository
	lines []domain.BalanceLine
}

func (b balanceLines) ListOwedLines(context.Context, string) ([]domain.BalanceLine, error) {
	return b.lines, nil
}

func balanceServer(status domain.CustomerStatus, lines ...domain.BalanceLine) *OrderServer {
	customer := domain.Customer{ID: uuid.New(), UserID: uuid.New(), Status: status, PendingSurcharge: decimal.RequireFromString("7.50")}
	return NewOrderServer(OrderUseCases{Balance: orderapp.NewGetMyBalance(payCustomers{customer: customer}, balanceLines{lines: lines})})
}

func TestGetMyBalanceShowsEachLineWithItsDueDate(t *testing.T) {
	started := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	srv := balanceServer(domain.StatusBlockedSurcharge,
		domain.BalanceLine{Kind: domain.BalanceLineSurcharge, ID: "s1", OrderID: "o1", Reason: "CLIENT_REFUSED", Amount: decimal.RequireFromString("3"), StartedAt: started},
		domain.BalanceLine{Kind: domain.BalanceLineRecidivism, ID: "r1", Reason: "RECIDIVISM", Amount: decimal.RequireFromString("4.5"), StartedAt: started.AddDate(0, 0, 2)},
	)

	res, err := srv.GetMyBalance(customerCtx(), &ordersv1.GetMyBalanceRequest{})

	if err != nil {
		t.Fatal(err)
	}
	if res.GetPendingSurcharge() != "7.50" || res.GetAccountStatus() != "BLOQUEADO_POR_SALDO" || len(res.GetLines()) != 2 {
		t.Fatalf("response = %+v", res)
	}
	first := res.GetLines()[0]
	if first.GetKind() != "SURCHARGE" || first.GetOrderId() != "o1" || first.GetReason() != "CLIENT_REFUSED" || first.GetAmount() != "3.00" ||
		first.GetStartedAt() != "2026-10-01T08:00:00Z" || first.GetDueAt() != "2026-10-15T08:00:00Z" {
		t.Fatalf("first line = %+v", first)
	}
	if res.GetLines()[1].GetKind() != "RECIDIVISM" || res.GetLines()[1].GetOrderId() != "" {
		t.Fatalf("second line = %+v", res.GetLines()[1])
	}
}

func TestGetMyBalanceErrorCodes(t *testing.T) {
	for name, tc := range map[string]struct {
		ctx  context.Context
		srv  *OrderServer
		want codes.Code
	}{
		"operator":    {asRole("OPERATOR"), balanceServer(domain.StatusActive), codes.PermissionDenied},
		"driver":      {asRole("DRIVER"), balanceServer(domain.StatusActive), codes.PermissionDenied},
		"no identity": {context.Background(), balanceServer(domain.StatusActive), codes.Unauthenticated},
		"not wired":   {customerCtx(), NewOrderServer(OrderUseCases{}), codes.Unimplemented},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := tc.srv.GetMyBalance(tc.ctx, &ordersv1.GetMyBalanceRequest{})
			wantCode(t, err, tc.want)
		})
	}
}
