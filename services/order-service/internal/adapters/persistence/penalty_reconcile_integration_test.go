//go:build integration

package persistence_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/persistence"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// nextStop gives every order its own sequence: stops are unique within a route, and one test places several orders on it.
var nextStop atomic.Int32

// placeInTransit deja un pedido con productos en una ruta y en transito, listo para cerrar su parada.
func placeInTransit(t *testing.T, store *persistence.Store, pool *pgxpool.Pool, customer string, p domain.Product, quantity int64) domain.Order {
	t.Helper()
	ctx := context.Background()
	placed, err := store.PlaceOrder(ctx, newOrder(t, customer), []domain.OrderLine{lineFor(t, p, quantity)}, nil, nil)
	require.NoError(t, err)
	_, err = persistence.NewOrderRepository(pool).AssignToRoute(ctx, []string{placed.ID}, func(o domain.Order) (domain.Order, error) {
		return o.AssignToRoute(testRoute, int(nextStop.Add(1)))
	})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE orders SET status = 'IN_TRANSIT' WHERE id = $1`, uuid.MustParse(placed.ID))
	require.NoError(t, err)
	return placed
}

// lostSurcharges simula el fallo que F1 recupera: el cierre de la parada funciona, el registro del recargo no.
type lostSurcharges struct {
	*persistence.SurchargeRepository
}

func (lostSurcharges) RecordSurchargeIncident(context.Context, domain.Surcharge) (domain.Surcharge, bool, error) {
	return domain.Surcharge{}, false, errors.New("db down")
}

func closeStopWith(t *testing.T, pool *pgxpool.Pool, orderID string, outcome domain.Status, reason string, surcharges ...ports.SurchargeRepository) {
	t.Helper()
	uc := orderapp.NewRecordStopOutcome(persistence.NewOrderRepository(pool))
	for _, s := range surcharges {
		uc = uc.WithSurcharges(s)
	}
	_, err := uc.Run(context.Background(), orderapp.RecordStopOutcomeCommand{
		Role: orderapp.RoleDriver, RouteID: testRoute, OrderID: orderID, Outcome: outcome, Reason: reason,
	})
	require.NoError(t, err)
}

func TestCerrarParadaGuardaElMotivoDeLaPenalidadSoloCuandoGeneraIncidencia(t *testing.T) {
	store, pool := setup(t)
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(50))

	cases := []struct {
		name    string
		outcome domain.Status
		reason  string
		wantRow bool
	}{
		{"rechazo del cliente", domain.StatusNotDelivered, "CLIENT_REFUSED", true},
		{"cliente ausente", domain.StatusNotDelivered, "CLIENT_ABSENT", true},
		{"fuera de ventana", domain.StatusNotDelivered, "OUT_OF_WINDOW", true},
		{"direccion incorrecta", domain.StatusNotDelivered, "WRONG_ADDRESS", true},
		{"motivo del negocio", domain.StatusNotDelivered, "PRODUCT_DAMAGED", false},
		{"pedido duplicado", domain.StatusNotDelivered, "DUPLICATE_ORDER", false},
		{"sin motivo", domain.StatusNotDelivered, "", false},
		{"entregado", domain.StatusDelivered, "", false},
		{"entregado con un motivo suelto", domain.StatusDelivered, "CLIENT_REFUSED", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			order := placeInTransit(t, store, pool, customer, p, 1)

			closeStopWith(t, pool, order.ID, tc.outcome, tc.reason)

			rows := countRows(t, pool, `SELECT count(*) FROM order_penalty_reasons WHERE order_id = $1`, uuid.MustParse(order.ID))
			if tc.wantRow {
				assert.Equal(t, 1, rows)
				var stored string
				require.NoError(t, pool.QueryRow(context.Background(), `SELECT reason FROM order_penalty_reasons WHERE order_id = $1`, uuid.MustParse(order.ID)).Scan(&stored))
				assert.Equal(t, tc.reason, stored)
			} else {
				assert.Equal(t, 0, rows)
			}
		})
	}
}

// El motivo y el estado se escriben juntos: si el cierre se rechaza no queda ningun motivo huerfano.
func TestUnCierreRechazadoNoDejaMotivoHuerfano(t *testing.T) {
	store, pool := setup(t)
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(10))
	placed, err := store.PlaceOrder(context.Background(), newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 1)}, nil, nil)
	require.NoError(t, err)
	// El pedido nunca fue asignado a una ruta: cerrar su parada es un error.
	_, err = orderapp.NewRecordStopOutcome(persistence.NewOrderRepository(pool)).Run(context.Background(), orderapp.RecordStopOutcomeCommand{
		Role: orderapp.RoleDriver, RouteID: testRoute, OrderID: placed.ID, Outcome: domain.StatusNotDelivered, Reason: "CLIENT_REFUSED",
	})

	require.Error(t, err)
	assert.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM order_penalty_reasons`))
}

func TestListarIncidenciasPerdidasDevuelveSoloLasQueNoTienenRecargo(t *testing.T) {
	store, pool := setup(t)
	surcharges := persistence.NewSurchargeRepository(pool)
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(50))
	ctx := context.Background()
	base := time.Date(2026, 10, 20, 8, 0, 0, 0, time.UTC)

	lostNew := placeInTransit(t, store, pool, customer, p, 1)
	lostOld := placeInTransit(t, store, pool, customer, p, 1)
	recorded := placeInTransit(t, store, pool, customer, p, 1)
	for order, at := range map[string]time.Time{lostNew.ID: base.Add(time.Hour), lostOld.ID: base, recorded.ID: base.Add(2 * time.Hour)} {
		_, err := pool.Exec(ctx, `INSERT INTO order_penalty_reasons (order_id, reason, recorded_at) VALUES ($1, 'CLIENT_REFUSED', $2)`, uuid.MustParse(order), at)
		require.NoError(t, err)
	}
	_, _, err := surcharges.RecordSurchargeIncident(ctx, pricedIncident(t, customer, recorded.ID, "CLIENT_REFUSED", "10.00"))
	require.NoError(t, err)
	// Un pedido sin motivo guardado (cerrado antes de F1) no aparece.
	placeInTransit(t, store, pool, customer, p, 1)

	got, err := surcharges.ListUnrecordedIncidents(ctx, 10)

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, lostOld.ID, got[0].OrderID, "el mas antiguo primero")
	assert.Equal(t, lostNew.ID, got[1].OrderID)
	assert.Equal(t, customer, got[0].CustomerID)
	assert.Equal(t, "CLIENT_REFUSED", got[0].Reason)
	assert.True(t, got[0].RecordedAt.Equal(base))

	limited, err := surcharges.ListUnrecordedIncidents(ctx, 1)
	require.NoError(t, err)
	require.Len(t, limited, 1)
	assert.Equal(t, lostOld.ID, limited[0].OrderID)
}

// De punta a punta con la base real: el cierre funciona, el registro del recargo se pierde, y el barrido lo recupera
// con el monto exacto, la fecha del cierre y el saldo del cliente, una sola vez.
func TestElBarridoRecuperaElRecargoQueSePerdioAlCerrarLaParada(t *testing.T) {
	store, pool := setup(t)
	ctx := context.Background()
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(50))
	const quantity = 3
	order := placeInTransit(t, store, pool, customer, p, quantity)
	surcharges := persistence.NewSurchargeRepository(pool)

	closeStopWith(t, pool, order.ID, domain.StatusNotDelivered, "CLIENT_REFUSED", lostSurcharges{surcharges})
	require.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM surcharges WHERE order_id = $1`, uuid.MustParse(order.ID)), "se perdio")
	require.True(t, balanceOf(t, pool, customer).IsZero())

	reconcile := orderapp.NewReconcileIncidents(surcharges, orderapp.NewIncidentRecorder(surcharges, nil))
	recovered, err := reconcile.Run(ctx)

	require.NoError(t, err)
	assert.Equal(t, 1, recovered)
	want := p.UnitPrice.Mul(decimal.NewFromInt(quantity)).Mul(decimal.RequireFromString("0.15")).Round(2)
	var amount decimal.Decimal
	var status string
	var sameDate bool
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT s.amount, s.status, s.created_at = p.recorded_at
		  FROM surcharges s JOIN order_penalty_reasons p USING (order_id)
		 WHERE s.order_id = $1`, uuid.MustParse(order.ID)).Scan(&amount, &status, &sameDate))
	assert.True(t, amount.Equal(want), "recargo %s, esperado %s", amount, want)
	assert.Equal(t, "PENDING", status)
	assert.True(t, sameDate, "conserva la fecha del cierre, no la del barrido")
	assert.True(t, balanceOf(t, pool, customer).Equal(want), "el saldo del cliente sube")

	again, err := reconcile.Run(ctx)
	require.NoError(t, err)
	assert.Zero(t, again, "repetirlo no crea otro")
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM surcharges WHERE order_id = $1`, uuid.MustParse(order.ID)))
	assert.True(t, balanceOf(t, pool, customer).Equal(want), "el saldo no se duplica")
}

// Un recargo que si se registro al cerrar la parada no lo toca el barrido.
func TestElBarridoNoTocaLoQueYaSeRegistro(t *testing.T) {
	store, pool := setup(t)
	customer := newCustomer(t, pool)
	p := newProduct(t, store, "Manzana", stock(50))
	order := placeInTransit(t, store, pool, customer, p, 2)
	surcharges := persistence.NewSurchargeRepository(pool)

	closeStopWith(t, pool, order.ID, domain.StatusNotDelivered, "CLIENT_ABSENT", surcharges)
	balance := balanceOf(t, pool, customer)
	require.True(t, balance.IsPositive())

	recovered, err := orderapp.NewReconcileIncidents(surcharges, orderapp.NewIncidentRecorder(surcharges, nil)).Run(context.Background())

	require.NoError(t, err)
	assert.Zero(t, recovered)
	assert.True(t, balanceOf(t, pool, customer).Equal(balance))
}
