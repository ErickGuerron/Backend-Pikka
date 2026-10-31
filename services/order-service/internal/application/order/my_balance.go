package order

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// GetBalanceCommand identifies the customer by the verified identity.
type GetBalanceCommand struct {
	UserID uuid.UUID
	Role   string
}

// Balance is what a customer sees about what they owe (ADR 0006, section 4): the total, the account
// status and each line with its reason.
type Balance struct {
	PendingSurcharge decimal.Decimal
	Status           domain.CustomerStatus
	Lines            []domain.BalanceLine
}

// GetMyBalance lets a customer see their balance. A blocked account can still see it, because that is how
// it knows what to pay to be unblocked (ADR 0006, section 5).
type GetMyBalance struct {
	customers ports.CustomerRepository
	balances  ports.BalanceRepository
}

func NewGetMyBalance(customers ports.CustomerRepository, balances ports.BalanceRepository) *GetMyBalance {
	return &GetMyBalance{customers: customers, balances: balances}
}

func (uc *GetMyBalance) Run(ctx context.Context, cmd GetBalanceCommand) (Balance, error) {
	if cmd.Role != RoleCustomer {
		return Balance{}, domain.ErrNotCustomer
	}
	customer, err := uc.customers.GetByUserID(ctx, cmd.UserID)
	if err != nil {
		return Balance{}, err
	}
	lines, err := uc.balances.ListOwedLines(ctx, customer.ID.String())
	if err != nil {
		return Balance{}, err
	}
	if lines == nil {
		lines = []domain.BalanceLine{}
	}
	return Balance{PendingSurcharge: customer.PendingSurcharge, Status: customer.Status, Lines: lines}, nil
}
