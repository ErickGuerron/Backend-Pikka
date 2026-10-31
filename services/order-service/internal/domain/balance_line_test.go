package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// The customer sees the day by which the line must be paid. It is the same day the sweep uses: the
// account is blocked only after that moment (ADR 0006, section 5).
func TestBalanceLineDueAtIsTwoWeeksAfterItStarted(t *testing.T) {
	line := domain.BalanceLine{StartedAt: time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)}

	assert.True(t, line.DueAt().Equal(time.Date(2026, 10, 15, 8, 0, 0, 0, time.UTC)))
}

func TestBalanceLineIsOverdueExactlyWhenTheSweepWouldBlock(t *testing.T) {
	line := domain.BalanceLine{StartedAt: time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)}
	due := line.DueAt()

	assert.False(t, domain.BalanceOverdue([]time.Time{line.StartedAt}, due))
	assert.True(t, domain.BalanceOverdue([]time.Time{line.StartedAt}, due.Add(time.Second)))
}
