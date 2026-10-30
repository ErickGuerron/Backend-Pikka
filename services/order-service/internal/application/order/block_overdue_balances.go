package order

import (
	"context"
	"sort"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// BlockOverdueBalances blocks the accounts whose unpaid balance is more than two weeks old (ADR 0006,
// section 5). It is safe to run any number of times: it only moves ACTIVO customers.
type BlockOverdueBalances struct {
	balances ports.BalanceRepository
	clock    func() time.Time
}

func NewBlockOverdueBalances(balances ports.BalanceRepository) *BlockOverdueBalances {
	return &BlockOverdueBalances{balances: balances, clock: time.Now}
}

// Run returns how many customers it blocked.
func (uc *BlockOverdueBalances) Run(ctx context.Context) (int, error) {
	obligations, err := uc.balances.ListActiveObligations(ctx)
	if err != nil {
		return 0, err
	}
	byCustomer := map[string][]time.Time{}
	for _, o := range obligations {
		byCustomer[o.CustomerID] = append(byCustomer[o.CustomerID], o.StartedAt)
	}
	now := uc.clock()
	var overdue []string
	for customerID, started := range byCustomer {
		if domain.BalanceOverdue(started, now) {
			overdue = append(overdue, customerID)
		}
	}
	if len(overdue) == 0 {
		return 0, nil
	}
	sort.Strings(overdue)
	return uc.balances.BlockCustomersForBalance(ctx, overdue)
}
