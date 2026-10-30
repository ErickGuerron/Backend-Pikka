package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

var day0 = time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)

func daysAfter(days ...int) []time.Time {
	out := make([]time.Time, 0, len(days))
	for _, d := range days {
		out = append(out, day0.AddDate(0, 0, d))
	}
	return out
}

// ADR 0006: la primera no entrega del cliente inicia un plazo de 2 semanas; la tercera dentro de ese
// plazo marca al cliente como reincidente.
func TestReincidenceReachedWithThreeInsideTheTwoWeeks(t *testing.T) {
	assert.True(t, domain.ReincidenceReached(daysAfter(0, 5, 13)))
	assert.True(t, domain.ReincidenceReached(daysAfter(0, 1, 2)))
}

func TestReincidenceDeadlineIsInclusive(t *testing.T) {
	assert.True(t, domain.ReincidenceReached(daysAfter(0, 7, 14)), "el dia 14 sigue dentro del plazo")
	assert.False(t, domain.ReincidenceReached(daysAfter(0, 7, 15)), "el dia 15 ya esta fuera")
}

func TestReincidenceNotReachedWithLessThanThree(t *testing.T) {
	assert.False(t, domain.ReincidenceReached(nil))
	assert.False(t, domain.ReincidenceReached(daysAfter(0)))
	assert.False(t, domain.ReincidenceReached(daysAfter(0, 1)))
}

func TestReincidenceDoesNotDependOnInputOrder(t *testing.T) {
	assert.True(t, domain.ReincidenceReached(daysAfter(13, 0, 5)))
}

// El plazo nace en la primera no entrega y no se desliza: 0, 10, 16 y 24 no son reincidencia, aunque
// 10, 16 y 24 cabrian en una ventana movil de 14 dias. Pasado el plazo, el dia 16 inicia uno nuevo.
func TestReincidenceWindowIsAnchoredOnTheFirstIncident(t *testing.T) {
	assert.False(t, domain.ReincidenceReached(daysAfter(0, 10, 16, 24)))
}

func TestReincidenceStartsANewWindowAfterTheFirstExpires(t *testing.T) {
	assert.True(t, domain.ReincidenceReached(daysAfter(0, 20, 21, 22)), "el dia 20 inicia un plazo nuevo con tres")
}
