package ports

import (
	"context"

	"github.com/shopspring/decimal"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// ChargeRequest asks the gateway to charge a customer. It has no card fields on purpose (ADR 0006).
type ChargeRequest struct {
	CustomerID string
	Amount     decimal.Decimal
	Method     domain.PaymentMethod
}

// RefundRequest asks the gateway to return a previous charge, identified by its reference.
type RefundRequest struct {
	Reference  string
	CustomerID string
	Amount     decimal.Decimal
	Method     domain.PaymentMethod
}

// PaymentGateway is the charge and refund boundary of the Order Service (ADR 0006, section 2). A real
// provider can replace the simulated adapter without changing the surcharge rules. A declined charge
// is a result (Status), not an error; errors mean the request itself was invalid.
type PaymentGateway interface {
	Charge(ctx context.Context, req ChargeRequest) (domain.PaymentAttempt, error)
	Refund(ctx context.Context, req RefundRequest) (domain.PaymentAttempt, error)
}

// PaymentAttemptRepository stores the ledger. Every attempt is recorded, approved or not.
type PaymentAttemptRepository interface {
	SavePaymentAttempt(ctx context.Context, attempt domain.PaymentAttempt) (domain.PaymentAttempt, error)
	ListPaymentAttempts(ctx context.Context, customerID string) ([]domain.PaymentAttempt, error)
}
