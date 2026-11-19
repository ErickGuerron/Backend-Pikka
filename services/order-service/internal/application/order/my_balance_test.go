package order

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

type linesFake struct {
	ports.BalanceRepository
	lines []domain.BalanceLine
	err   error
	asked string
}

func (f *linesFake) ListOwedLines(_ context.Context, customerID string) ([]domain.BalanceLine, error) {
	f.asked = customerID
	return f.lines, f.err
}

func newBalanceView(owed string, status domain.CustomerStatus, repo *linesFake) (*GetMyBalance, domain.Customer) {
	customer := activeCustomer()
	customer.Status = status
	customer.PendingSurcharge = decimal.RequireFromString(owed)
	customers := &fakeCustomers{byUser: map[uuid.UUID]domain.Customer{customer.UserID: customer}}
	return NewGetMyBalance(customers, repo), customer
}

func TestGetMyBalanceShowsTheBalanceAndEachLineWithItsReason(t *testing.T) {
	started := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	repo := &linesFake{lines: []domain.BalanceLine{
		{Kind: domain.BalanceLineSurcharge, ID: "s1", OrderID: "o1", Reason: "CLIENT_REFUSED", Amount: decimal.RequireFromString("3.00"), StartedAt: started},
		{Kind: domain.BalanceLineRecidivism, ID: "r1", Reason: "RECIDIVISM", Amount: decimal.RequireFromString("4.50"), StartedAt: started.AddDate(0, 0, 2)},
	}}
	uc, customer := newBalanceView("7.50", domain.StatusActive, repo)

	got, err := uc.Run(context.Background(), GetBalanceCommand{UserID: customer.UserID, Role: RoleCustomer})

	if err != nil {
		t.Fatal(err)
	}
	if got.PendingSurcharge.StringFixed(2) != "7.50" || got.Status != domain.StatusActive || len(got.Lines) != 2 {
		t.Fatalf("balance = %+v", got)
	}
	if repo.asked != customer.ID.String() {
		t.Fatalf("lines asked for %q, want the authenticated customer %q", repo.asked, customer.ID)
	}
}

// ADR 0006 sección 5: la cuenta bloqueada puede seguir entrando a la app y ver el saldo.
func TestGetMyBalanceStillWorksWhenTheAccountIsBlocked(t *testing.T) {
	uc, customer := newBalanceView("3.00", domain.StatusBlockedSurcharge, &linesFake{})

	got, err := uc.Run(context.Background(), GetBalanceCommand{UserID: customer.UserID, Role: RoleCustomer})

	if err != nil || got.Status != domain.StatusBlockedSurcharge {
		t.Fatalf("status = %s, err = %v", got.Status, err)
	}
}

func TestGetMyBalanceWithNothingOwedHasNoLines(t *testing.T) {
	uc, customer := newBalanceView("0", domain.StatusActive, &linesFake{})

	got, err := uc.Run(context.Background(), GetBalanceCommand{UserID: customer.UserID, Role: RoleCustomer})

	if err != nil || got.Lines == nil || len(got.Lines) != 0 || !got.PendingSurcharge.IsZero() {
		t.Fatalf("balance = %+v, err = %v", got, err)
	}
}

func TestGetMyBalanceIsOnlyForCustomers(t *testing.T) {
	uc, customer := newBalanceView("3.00", domain.StatusActive, &linesFake{})
	for _, role := range []string{RoleOperator, RoleAdmin, RoleDriver} {
		if _, err := uc.Run(context.Background(), GetBalanceCommand{UserID: customer.UserID, Role: role}); !errors.Is(err, domain.ErrNotCustomer) {
			t.Fatalf("%s: err = %v, want ErrNotCustomer", role, err)
		}
	}
}

func TestGetMyBalanceReturnsRepositoryErrors(t *testing.T) {
	uc, customer := newBalanceView("3.00", domain.StatusActive, &linesFake{err: errors.New("db down")})
	if _, err := uc.Run(context.Background(), GetBalanceCommand{UserID: customer.UserID, Role: RoleCustomer}); err == nil {
		t.Fatal("want the repository error")
	}
}
