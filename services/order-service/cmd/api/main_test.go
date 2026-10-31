package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

type sweepRepo struct {
	ports.BalanceRepository
	listed  atomic.Int32
	listErr error
	late    bool
}

func (r *sweepRepo) ListActiveObligations(context.Context) ([]domain.BalanceObligation, error) {
	r.listed.Add(1)
	if r.listErr != nil {
		return nil, r.listErr
	}
	if r.late {
		return []domain.BalanceObligation{{CustomerID: "c-1", StartedAt: time.Now().AddDate(0, 0, -30)}}, nil
	}
	return nil, nil
}

func (r *sweepRepo) BlockCustomersForBalance(_ context.Context, ids []string) (int, error) {
	return len(ids), nil
}

// noLostIncidents is a surcharge store with nothing to recover, so the reconcile step of the loop is a no-op here.
type noLostIncidents struct{ ports.SurchargeRepository }

func (noLostIncidents) ListUnrecordedIncidents(context.Context, int) ([]domain.UnrecordedIncident, error) {
	return nil, nil
}

// runSweep runs the loop until it has passed twice (the start pass and one tick), then stops it and returns its log.
func runSweep(t *testing.T, repo *sweepRepo) string {
	t.Helper()
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runBalanceSweep(ctx, log, orderapp.NewReconcileIncidents(noLostIncidents{}, nil), orderapp.NewBlockOverdueBalances(repo), time.Millisecond)
		close(done)
	}()
	deadline := time.After(5 * time.Second)
	for repo.listed.Load() < 2 {
		select {
		case <-deadline:
			t.Fatal("the sweep did not run on its first tick")
		case <-time.After(time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-deadline:
		t.Fatal("the sweep did not stop when its context ended")
	}
	return buf.String()
}

func TestSweepRunsAtStartAndOnEveryTickAndStopsWithItsContext(t *testing.T) {
	if out := runSweep(t, &sweepRepo{}); strings.Contains(out, "ERROR") {
		t.Fatalf("a quiet pass must not log an error: %s", out)
	}
}

func TestSweepLogsTheAccountsItBlocked(t *testing.T) {
	if out := runSweep(t, &sweepRepo{late: true}); !strings.Contains(out, "accounts blocked for an unpaid balance") {
		t.Fatalf("missing the block warning: %s", out)
	}
}

func TestSweepFailureIsLoggedAndTheNextPassRetries(t *testing.T) {
	repo := &sweepRepo{listErr: errors.New("db down")}
	if out := runSweep(t, repo); !strings.Contains(out, "balance sweep failed") {
		t.Fatalf("missing the failure log: %s", out)
	}
}
