//go:build integration

package persistence_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/persistence"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// Pruebas obligatorias del ADR 0006 ("Consecuencias"), contra la base real y de punta a punta.

// "el recargo es exactamente 15 % del precio base y se aplica una sola vez por incidencia".
// Se recorre el camino real: pedido con productos en una ruta, en transito, el conductor lo cierra como
// rechazado por el cliente. El precio base se calcula aqui a mano, sin usar el codigo bajo prueba.
func TestADR0006_RecargoEsExactamenteElQuincePorCientoYSeAplicaUnaSolaVez(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(20))
	const quantity = 3
	placed, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, quantity)}, nil, nil)
	require.NoError(t, err)

	orders := persistence.NewOrderRepository(pool)
	_, err = orders.AssignToRoute(ctx, []string{placed.ID}, func(o domain.Order) (domain.Order, error) { return o.AssignToRoute(testRoute, 1) })
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE orders SET status = 'IN_TRANSIT' WHERE id = $1`, uuid.MustParse(placed.ID))
	require.NoError(t, err)

	base := p.UnitPrice.Mul(decimal.NewFromInt(quantity))
	want := base.Mul(decimal.RequireFromString("0.15")).Round(2)
	require.True(t, want.IsPositive())

	surcharges := persistence.NewSurchargeRepository(pool)
	closeStop := orderapp.NewRecordStopOutcome(orders).WithSurcharges(surcharges)
	cmd := orderapp.RecordStopOutcomeCommand{Role: orderapp.RoleDriver, RouteID: testRoute, OrderID: placed.ID, Outcome: domain.StatusNotDelivered, Reason: "CLIENT_REFUSED"}

	_, err = closeStop.Run(ctx, cmd)
	require.NoError(t, err)

	var amount decimal.Decimal
	require.NoError(t, pool.QueryRow(ctx, `SELECT amount FROM surcharges WHERE order_id = $1`, uuid.MustParse(placed.ID)).Scan(&amount))
	assert.True(t, amount.Equal(want), "recargo %s, esperado exactamente 15 %% de %s = %s", amount, base, want)
	assert.True(t, balanceOf(t, pool, customer).Equal(want), "el saldo es ese recargo")

	// Una segunda vez por el mismo pedido: la parada ya esta cerrada y nada se vuelve a cobrar.
	_, err = closeStop.Run(ctx, cmd)
	assert.Error(t, err)
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM surcharges WHERE order_id = $1`, uuid.MustParse(placed.ID)))
	assert.True(t, balanceOf(t, pool, customer).Equal(want), "el saldo no se duplica")

	// Y aunque el evento llegara repetido a la base, el pedido es unico por incidencia.
	_, created, err := surcharges.RecordSurchargeIncident(ctx, pricedIncident(t, customer, placed.ID, "CLIENT_REFUSED", base.String()))
	require.NoError(t, err)
	assert.False(t, created)
	assert.True(t, balanceOf(t, pool, customer).Equal(want))
}

// "ningun log, respuesta ni tabla contiene datos de tarjeta": ninguna columna de ninguna tabla del esquema.
func TestADR0006_NingunaTablaDelEsquemaGuardaDatosDeTarjeta(t *testing.T) {
	_, pool := setup(t)
	rows, err := pool.Query(context.Background(), `
		SELECT table_name, column_name FROM information_schema.columns WHERE table_schema = current_schema()`)
	require.NoError(t, err)
	defer rows.Close()

	checked := 0
	for rows.Next() {
		var table, column string
		require.NoError(t, rows.Scan(&table, &column))
		checked++
		assert.False(t, cardDataColumn.MatchString(column), "la columna %s.%s parece guardar datos de tarjeta", table, column)
	}
	require.NoError(t, rows.Err())
	assert.Greater(t, checked, 50, "se reviso todo el esquema, no una tabla")
}

// La prueba anterior detecta lo que debe: sensibilidad del patron.
func TestADR0006_ElDetectorDeColumnasDeTarjetaFunciona(t *testing.T) {
	for _, bad := range []string{"card_number", "cardNumber", "cvv", "cvc", "pan", "card_holder", "expiry_date", "expiration_month", "last4", "last_four", "security_code", "card_brand"} {
		assert.True(t, cardDataColumn.MatchString(bad), bad)
	}
	for _, ok := range []string{"amount", "method", "status", "reference", "created_at", "pending_surcharge", "unit_price", "applied_by", "paid_at", "recidivist_at", "panel_id_no"} {
		assert.False(t, cardDataColumn.MatchString(ok), ok)
	}
}

var cardDataColumn = regexp.MustCompile(`(?i)^(card.*|.*card_?(number|holder|brand|expir.*)|cvv2?|cvc2?|pan|last_?4|last_?four|expir(y|ation).*|security_?code)$`)

// "el cobro simulado nunca se presenta como cargo real": la base solo acepta estados SIMULATED_*.
func TestADR0006_PaymentAttemptsSoloAceptaEstadosSimulados(t *testing.T) {
	_, pool := setup(t)
	customer := newCustomer(t, pool)
	insert := func(status, reference string) error {
		_, err := pool.Exec(context.Background(),
			`INSERT INTO payment_attempts (customer_id, amount, method, status, reference) VALUES ($1, 5, 'CARD', $2, $3)`, customer, status, reference)
		return err
	}

	require.NoError(t, insert("SIMULATED_APPROVED", "ref-ok-1"))
	require.NoError(t, insert("SIMULATED_DECLINED", "ref-ok-2"))
	for i, real := range []string{"APPROVED", "DECLINED", "PAID", "CAPTURED", "approved"} {
		assert.Error(t, insert(real, "ref-bad-"+string(rune('a'+i))), "%q no es un estado simulado", real)
	}
}
