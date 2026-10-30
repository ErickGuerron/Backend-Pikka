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

type gatewayFake struct {
	charged  []ports.ChargeRequest
	refunded []ports.RefundRequest
	status   domain.PaymentStatus
}

func (g *gatewayFake) Charge(_ context.Context, req ports.ChargeRequest) (domain.PaymentAttempt, error) {
	g.charged = append(g.charged, req)
	status := g.status
	if status == "" {
		status = domain.PaymentApproved
	}
	return domain.NewPaymentAttempt(req.CustomerID, req.Amount, req.Method, status, "SIM-"+uuid.NewString(), time.Now())
}

func (g *gatewayFake) Refund(_ context.Context, req ports.RefundRequest) (domain.PaymentAttempt, error) {
	g.refunded = append(g.refunded, req)
	return domain.NewPaymentAttempt(req.CustomerID, req.Amount, req.Method, domain.PaymentApproved, "SIM-REFUND-"+uuid.NewString(), time.Now())
}

type attemptsFake struct {
	ports.PaymentAttemptRepository
	saved   []domain.PaymentAttempt
	saveErr error
}

func (a *attemptsFake) SavePaymentAttempt(_ context.Context, p domain.PaymentAttempt) (domain.PaymentAttempt, error) {
	if a.saveErr != nil {
		return domain.PaymentAttempt{}, a.saveErr
	}
	p.ID = uuid.NewString()
	a.saved = append(a.saved, p)
	return p, nil
}

type settleFake struct {
	ports.BalanceRepository
	calls     int
	amount    decimal.Decimal
	unblocked bool
	err       error
}

func (s *settleFake) SettleBalance(_ context.Context, _ string, amount decimal.Decimal, _ time.Time) (bool, error) {
	s.calls++
	s.amount = amount
	return s.unblocked, s.err
}

type payFixture struct {
	uc       *PayBalance
	gateway  *gatewayFake
	attempts *attemptsFake
	settle   *settleFake
	customer domain.Customer
}

func newPay(owed string, status domain.CustomerStatus) payFixture {
	customer := activeCustomer()
	customer.Status = status
	customer.PendingSurcharge = decimal.RequireFromString(owed)
	f := payFixture{gateway: &gatewayFake{}, attempts: &attemptsFake{}, settle: &settleFake{unblocked: status == domain.StatusBlockedSurcharge}, customer: customer}
	customers := &fakeCustomers{byUser: map[uuid.UUID]domain.Customer{customer.UserID: customer}}
	f.uc = NewPayBalance(customers, f.gateway, f.attempts, f.settle)
	return f
}

func (f payFixture) pay() (PaymentResult, error) {
	return f.uc.Run(context.Background(), PayBalanceCommand{UserID: f.customer.UserID, Role: RoleCustomer})
}

// ADR 0006 sección 5: se paga el saldo completo con el cobro simulado y la cuenta vuelve a ACTIVO en ese momento.
func TestPayBalanceChargesTheFullBalanceAndUnblocks(t *testing.T) {
	f := newPay("7.50", domain.StatusBlockedSurcharge)

	got, err := f.pay()

	if err != nil {
		t.Fatal(err)
	}
	if len(f.gateway.charged) != 1 || f.gateway.charged[0].Amount.StringFixed(2) != "7.50" || f.gateway.charged[0].Method != domain.PaymentCard {
		t.Fatalf("charged = %+v", f.gateway.charged)
	}
	if len(f.attempts.saved) != 1 || f.attempts.saved[0].Status != domain.PaymentApproved {
		t.Fatalf("attempts = %+v, want one SIMULATED_APPROVED", f.attempts.saved)
	}
	if f.settle.calls != 1 || f.settle.amount.StringFixed(2) != "7.50" {
		t.Fatalf("settle calls = %d amount = %s", f.settle.calls, f.settle.amount)
	}
	if !got.Approved || !got.Unblocked || got.Attempt.Status != domain.PaymentApproved {
		t.Fatalf("result = %+v", got)
	}
}

func TestPayBalanceWhenActiveDoesNotReportUnblocked(t *testing.T) {
	f := newPay("3.00", domain.StatusActive)

	got, err := f.pay()

	if err != nil || !got.Approved || got.Unblocked {
		t.Fatalf("result = %+v, err = %v", got, err)
	}
}

// A declined simulated card is recorded and leaves the balance and the account exactly as they were.
func TestPayBalanceDeclinedIsRecordedAndChangesNothing(t *testing.T) {
	f := newPay("7.99", domain.StatusBlockedSurcharge)
	f.gateway.status = domain.PaymentDeclined

	got, err := f.pay()

	if err != nil {
		t.Fatal(err)
	}
	if got.Approved || got.Unblocked || got.Attempt.Status != domain.PaymentDeclined {
		t.Fatalf("result = %+v", got)
	}
	if len(f.attempts.saved) != 1 || f.attempts.saved[0].Status != domain.PaymentDeclined {
		t.Fatalf("attempts = %+v, want the declined one recorded", f.attempts.saved)
	}
	if f.settle.calls != 0 {
		t.Fatalf("settle called %d times after a decline", f.settle.calls)
	}
}

func TestPayBalanceWithNothingOwedDoesNotCharge(t *testing.T) {
	f := newPay("0", domain.StatusActive)

	_, err := f.pay()

	if !errors.Is(err, domain.ErrNothingToPay) {
		t.Fatalf("err = %v, want ErrNothingToPay", err)
	}
	if len(f.gateway.charged) != 0 || len(f.attempts.saved) != 0 {
		t.Fatalf("charged %d, recorded %d, want none", len(f.gateway.charged), len(f.attempts.saved))
	}
}

func TestPayBalanceIsOnlyForCustomers(t *testing.T) {
	f := newPay("7.50", domain.StatusActive)

	_, err := f.uc.Run(context.Background(), PayBalanceCommand{UserID: f.customer.UserID, Role: RoleOperator})

	if !errors.Is(err, domain.ErrNotCustomer) || len(f.gateway.charged) != 0 {
		t.Fatalf("err = %v, charged = %d", err, len(f.gateway.charged))
	}
}

// If the balance moved between the charge and the settlement, the money is given back and recorded.
func TestPayBalanceRefundsWhenTheBalanceChanged(t *testing.T) {
	f := newPay("7.50", domain.StatusBlockedSurcharge)
	f.settle.err = domain.ErrBalanceChanged

	_, err := f.pay()

	if !errors.Is(err, domain.ErrBalanceChanged) {
		t.Fatalf("err = %v, want ErrBalanceChanged", err)
	}
	if len(f.gateway.refunded) != 1 || f.gateway.refunded[0].Amount.StringFixed(2) != "7.50" {
		t.Fatalf("refunds = %+v", f.gateway.refunded)
	}
	if len(f.attempts.saved) != 2 {
		t.Fatalf("attempts = %d, want the charge and its refund", len(f.attempts.saved))
	}
}

func TestPayBalanceStopsIfTheAttemptCannotBeRecorded(t *testing.T) {
	f := newPay("7.50", domain.StatusBlockedSurcharge)
	f.attempts.saveErr = errors.New("db down")

	_, err := f.pay()

	if err == nil || f.settle.calls != 0 {
		t.Fatalf("err = %v, settle calls = %d", err, f.settle.calls)
	}
}
