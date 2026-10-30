package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

var started = time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)

// ADR 0006 sección 5: "si pasadas 2 semanas el saldo sigue sin pagar, la cuenta pasa a BLOQUEADO_POR_SALDO".
func TestBalanceOverdueOnlyAfterTwoWeeks(t *testing.T) {
	exactly := started.AddDate(0, 0, 14)
	assert.False(t, domain.BalanceOverdue([]time.Time{started}, exactly), "a las 2 semanas justas todavia no ha pasado el plazo")
	assert.True(t, domain.BalanceOverdue([]time.Time{started}, exactly.Add(time.Second)), "un segundo despues ya paso")
	assert.False(t, domain.BalanceOverdue([]time.Time{started}, started.AddDate(0, 0, 13)))
}

// Con varios recargos pendientes manda el mas antiguo: el plazo del primero no se renueva por los nuevos.
func TestBalanceOverdueLooksAtTheOldestObligation(t *testing.T) {
	now := started.AddDate(0, 0, 20)
	fresh := started.AddDate(0, 0, 19)
	assert.True(t, domain.BalanceOverdue([]time.Time{fresh, started}, now))
	assert.False(t, domain.BalanceOverdue([]time.Time{fresh}, now))
}

func TestBalanceOverdueWithNothingPendingIsFalse(t *testing.T) {
	assert.False(t, domain.BalanceOverdue(nil, started.AddDate(1, 0, 0)))
}
