package domain

import (
	"testing"
	"time"
)

func TestOnlyAnUndisputedSurchargeLineCanStillBeDisputedAndUntilTwoHoursAfterItStarted(t *testing.T) {
	start := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	line := BalanceLine{Kind: BalanceLineSurcharge, StartedAt: start}

	if got := line.DisputableUntil(); !got.Equal(start.Add(DisputeWindow)) {
		t.Fatalf("until = %v, want %v", got, start.Add(DisputeWindow))
	}

	for name, l := range map[string]BalanceLine{
		"already disputed (open)":      {Kind: BalanceLineSurcharge, StartedAt: start, DisputeStatus: string(DisputeOpen)},
		"already ruled against":        {Kind: BalanceLineSurcharge, StartedAt: start, DisputeStatus: string(DisputeAgainst)},
		"an extra (recidivism) charge": {Kind: BalanceLineRecidivism, StartedAt: start},
		"a reassignment fee":           {Kind: BalanceLineReassignment, StartedAt: start},
	} {
		if got := l.DisputableUntil(); !got.IsZero() {
			t.Fatalf("%s: until = %v, want zero (not disputable)", name, got)
		}
	}
}
