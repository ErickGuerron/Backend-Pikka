package order

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// PayBalanceCommand identifies the paying customer by the verified identity, never by the body.
type PayBalanceCommand struct {
	UserID uuid.UUID
	Role   string
}

// PaymentResult tells how the simulated charge ended. A declined charge is a result, not an error:
// the attempt is recorded and the balance and the account stay as they were.
type PaymentResult struct {
	Attempt   domain.PaymentAttempt
	Approved  bool
	Unblocked bool
}

// PayBalance lets a customer pay the whole pending balance with the simulated card (ADR 0006, section 5).
// An approved payment settles the balance and, in that same moment, returns a balance-blocked account
// to ACTIVO. Only the full balance can be paid, so there is no amount to send.
type PayBalance struct {
	customers ports.CustomerRepository
	gateway   ports.PaymentGateway
	attempts  ports.PaymentAttemptRepository
	balances  ports.BalanceRepository
	clock     func() time.Time
}

func NewPayBalance(customers ports.CustomerRepository, gateway ports.PaymentGateway, attempts ports.PaymentAttemptRepository, balances ports.BalanceRepository) *PayBalance {
	return &PayBalance{customers: customers, gateway: gateway, attempts: attempts, balances: balances, clock: time.Now}
}

// Run charges, records the attempt and, if approved, settles. If the settlement fails after the charge
// (for example the balance moved), the charge is refunded and the refund recorded, so no money is kept
// without the balance being paid.
func (uc *PayBalance) Run(ctx context.Context, cmd PayBalanceCommand) (PaymentResult, error) {
	if cmd.Role != RoleCustomer {
		return PaymentResult{}, domain.ErrNotCustomer
	}
	customer, err := uc.customers.GetByUserID(ctx, cmd.UserID)
	if err != nil {
		return PaymentResult{}, err
	}
	amount := customer.PendingSurcharge.Round(2)
	if !amount.IsPositive() {
		return PaymentResult{}, domain.ErrNothingToPay
	}
	customerID := customer.ID.String()

	attempt, err := uc.gateway.Charge(ctx, ports.ChargeRequest{CustomerID: customerID, Amount: amount, Method: domain.PaymentCard})
	if err != nil {
		return PaymentResult{}, err
	}
	saved, err := uc.attempts.SavePaymentAttempt(ctx, attempt)
	if err != nil {
		return PaymentResult{}, fmt.Errorf("record payment attempt: %w", err)
	}
	if saved.Status != domain.PaymentApproved {
		return PaymentResult{Attempt: saved}, nil
	}

	unblocked, err := uc.balances.SettleBalance(ctx, customerID, amount, uc.clock())
	if err != nil {
		uc.refund(ctx, customerID, amount, saved.Reference)
		return PaymentResult{}, err
	}
	return PaymentResult{Attempt: saved, Approved: true, Unblocked: unblocked}, nil
}

// refund gives the money back after a failed settlement. A failure here is logged with the reference,
// because someone has to reconcile that payment by hand.
func (uc *PayBalance) refund(ctx context.Context, customerID string, amount decimal.Decimal, reference string) {
	back, err := uc.gateway.Refund(ctx, ports.RefundRequest{Reference: reference, CustomerID: customerID, Amount: amount, Method: domain.PaymentCard})
	if err == nil {
		_, err = uc.attempts.SavePaymentAttempt(ctx, back)
	}
	if err != nil {
		slog.Default().Error("payment refund not recorded; reconcile by hand", "customer", customerID, "reference", reference, "error", err)
	}
}
