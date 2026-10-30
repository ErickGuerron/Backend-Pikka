package order

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

type balanceRepoFake struct {
	ports.BalanceRepository
	obligations []domain.BalanceObligation
	listErr     error
	blockErr    error
	blocked     []string
	blockCalls  int
}

func (f *balanceRepoFake) ListActiveObligations(context.Context) ([]domain.BalanceObligation, error) {
	return f.obligations, f.listErr
}

func (f *balanceRepoFake) BlockCustomersForBalance(_ context.Context, ids []string) (int, error) {
	f.blockCalls++
	if f.blockErr != nil {
		return 0, f.blockErr
	}
	f.blocked = append([]string(nil), ids...)
	sort.Strings(f.blocked)
	return len(ids), nil
}

var sweepNow = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

func sweep(repo *balanceRepoFake) (int, error) {
	uc := NewBlockOverdueBalances(repo)
	uc.clock = func() time.Time { return sweepNow }
	return uc.Run(context.Background())
}

func daysAgo(n int) time.Time { return sweepNow.AddDate(0, 0, -n) }

func TestSweepBlocksOnlyCustomersWithAnOverdueObligation(t *testing.T) {
	repo := &balanceRepoFake{obligations: []domain.BalanceObligation{
		{CustomerID: "late", StartedAt: daysAgo(20)},
		{CustomerID: "fresh", StartedAt: daysAgo(3)},
		{CustomerID: "edge", StartedAt: daysAgo(14)},
	}}

	n, err := sweep(repo)

	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(repo.blocked) != 1 || repo.blocked[0] != "late" {
		t.Fatalf("blocked = %v (n=%d), want [late]", repo.blocked, n)
	}
}

// One customer with a fresh and an old obligation is judged by the old one, and listed once.
func TestSweepJudgesACustomerByItsOldestObligation(t *testing.T) {
	repo := &balanceRepoFake{obligations: []domain.BalanceObligation{
		{CustomerID: "c1", StartedAt: daysAgo(2)},
		{CustomerID: "c1", StartedAt: daysAgo(30)},
		{CustomerID: "c1", StartedAt: daysAgo(16)},
	}}

	if _, err := sweep(repo); err != nil {
		t.Fatal(err)
	}
	if len(repo.blocked) != 1 || repo.blocked[0] != "c1" {
		t.Fatalf("blocked = %v, want [c1] once", repo.blocked)
	}
}

func TestSweepWithNothingOverdueDoesNotWrite(t *testing.T) {
	repo := &balanceRepoFake{obligations: []domain.BalanceObligation{{CustomerID: "fresh", StartedAt: daysAgo(1)}}}

	n, err := sweep(repo)

	if err != nil || n != 0 || repo.blockCalls != 0 {
		t.Fatalf("n = %d, err = %v, block calls = %d", n, err, repo.blockCalls)
	}
}

func TestSweepReturnsRepositoryErrors(t *testing.T) {
	if _, err := sweep(&balanceRepoFake{listErr: errors.New("db down")}); err == nil {
		t.Fatal("want the list error")
	}
	repo := &balanceRepoFake{obligations: []domain.BalanceObligation{{CustomerID: "late", StartedAt: daysAgo(20)}}, blockErr: errors.New("db down")}
	if _, err := sweep(repo); err == nil {
		t.Fatal("want the block error")
	}
}
