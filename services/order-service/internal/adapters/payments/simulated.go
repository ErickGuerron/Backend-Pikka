// Package payments holds PaymentGateway adapters. The MVP only has the simulated one (ADR 0006).
package payments

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// declinedCents is the test rule: a CARD charge whose amount ends in .99 is declined. Cash is
// confirmed by the driver, so it is never declined.
var declinedCents = decimal.NewFromInt(99)

// Simulated approves or declines by a fixed rule. It makes no network call and never sees card data.
type Simulated struct {
	now func() time.Time
}

var _ ports.PaymentGateway = (*Simulated)(nil)

func NewSimulated(now func() time.Time) *Simulated { return &Simulated{now: now} }

func (s *Simulated) Charge(_ context.Context, req ports.ChargeRequest) (domain.PaymentAttempt, error) {
	status := domain.PaymentApproved
	if req.Method == domain.PaymentCard && declinedByRule(req.Amount) {
		status = domain.PaymentDeclined
	}
	return s.attempt(req.CustomerID, req.Amount, req.Method, status, "SIM-"+uuid.NewString())
}

func (s *Simulated) Refund(_ context.Context, req ports.RefundRequest) (domain.PaymentAttempt, error) {
	if req.Reference == "" {
		return domain.PaymentAttempt{}, domain.ErrInvalidPaymentReference
	}
	return s.attempt(req.CustomerID, req.Amount, req.Method, domain.PaymentApproved, "SIM-REFUND-"+uuid.NewString())
}

func (s *Simulated) attempt(customerID string, amount decimal.Decimal, method domain.PaymentMethod, status domain.PaymentStatus, reference string) (domain.PaymentAttempt, error) {
	if _, err := domain.ParsePaymentMethod(string(method)); err != nil {
		return domain.PaymentAttempt{}, fmt.Errorf("simulated payment: %w", err)
	}
	return domain.NewPaymentAttempt(customerID, amount, method, status, reference, s.now())
}

func declinedByRule(amount decimal.Decimal) bool {
	cents := amount.Round(2).Mul(decimal.NewFromInt(100)).Mod(decimal.NewFromInt(100))
	return cents.Equal(declinedCents)
}
