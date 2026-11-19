//go:build integration

package persistence_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/persistence"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

const reassignmentAdmin = "6f9619ff-8b86-d011-b42d-00c04fc964ff"

func setFee(t *testing.T, repo *persistence.ReassignmentRepository, amount string) domain.ReassignmentFee {
	t.Helper()
	fee, err := domain.NewReassignmentFee(decimal.RequireFromString(amount), reassignmentAdmin, "Sube la gasolina", time.Now())
	require.NoError(t, err)
	saved, err := repo.SetReassignmentFee(context.Background(), fee)
	require.NoError(t, err)
	return saved
}

func buildReassignment(orderID string) func(string, *domain.ReassignmentFee) (domain.ReassignmentCharge, error) {
	return func(customerID string, fee *domain.ReassignmentFee) (domain.ReassignmentCharge, error) {
		return domain.NewReassignmentCharge(customerID, orderID, fee, reassignmentAdmin, "Pidió cambiar la entrega con el repartidor en camino", time.Now())
	}
}

func placedOrder(t *testing.T, store *persistence.Store, pool *pgxpool.Pool, customer string) string {
	t.Helper()
	product := newProduct(t, store, "Pan", stock(5))
	placed, err := store.PlaceOrder(context.Background(), newOrder(t, customer), []domain.OrderLine{lineFor(t, product, 1)}, nil, nil)
	require.NoError(t, err)
	return placed.ID
}

func TestSinTarifaDeReasignacionNoSeCobraNada(t *testing.T) {
	store, pool := setup(t)
	repo := persistence.NewReassignmentRepository(pool)
	customer := newCustomer(t, pool)
	order := placedOrder(t, store, pool, customer)

	_, err := repo.ApplyReassignmentCharge(context.Background(), order, buildReassignment(order))

	assert.ErrorIs(t, err, domain.ErrReassignmentFeeNotSet)
	assert.True(t, balanceOf(t, pool, customer).IsZero())
	assert.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM reassignment_charges`))
}

func TestLaTarifaVigenteEsLaUltimaYCadaCambioQuedaAuditado(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewReassignmentRepository(pool)
	setFee(t, repo, "2.00")
	time.Sleep(5 * time.Millisecond)
	second := setFee(t, repo, "3.50")

	current, err := repo.CurrentReassignmentFee(context.Background())

	require.NoError(t, err)
	assert.Equal(t, second.ID, current.ID)
	assert.True(t, current.Amount.Equal(decimal.RequireFromString("3.50")))
	assert.Equal(t, reassignmentAdmin, current.SetBy)
	assert.Equal(t, 2, countRows(t, pool, `SELECT count(*) FROM reassignment_fees`))
}

func TestCobroPorReasignacionSumaAlSaldoConLaTarifaVigente(t *testing.T) {
	store, pool := setup(t)
	repo := persistence.NewReassignmentRepository(pool)
	customer := newCustomer(t, pool)
	order := placedOrder(t, store, pool, customer)
	setFee(t, repo, "2.50")

	saved, err := repo.ApplyReassignmentCharge(context.Background(), order, buildReassignment(order))

	require.NoError(t, err)
	assert.NotEmpty(t, saved.ID)
	assert.Equal(t, customer, saved.CustomerID)
	assert.True(t, balanceOf(t, pool, customer).Equal(decimal.RequireFromString("2.50")))
	assert.Equal(t, 1, countRows(t, pool, `SELECT count(*) FROM reassignment_charges WHERE order_id = $1 AND paid_at IS NULL`, order))
}

func TestSegundoCobroPorReasignacionDelMismoPedidoSeRechazaYNoSumaSaldo(t *testing.T) {
	store, pool := setup(t)
	repo := persistence.NewReassignmentRepository(pool)
	customer := newCustomer(t, pool)
	order := placedOrder(t, store, pool, customer)
	setFee(t, repo, "2.50")
	_, err := repo.ApplyReassignmentCharge(context.Background(), order, buildReassignment(order))
	require.NoError(t, err)

	_, err = repo.ApplyReassignmentCharge(context.Background(), order, buildReassignment(order))

	assert.ErrorIs(t, err, domain.ErrReassignmentChargeExists)
	assert.True(t, balanceOf(t, pool, customer).Equal(decimal.RequireFromString("2.50")))
}

func TestCobroPorReasignacionDePedidoInexistente(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewReassignmentRepository(pool)
	setFee(t, repo, "2.50")
	ghost := uuid.NewString()

	_, err := repo.ApplyReassignmentCharge(context.Background(), ghost, buildReassignment(ghost))

	assert.ErrorIs(t, err, domain.ErrOrderNotFound)
}

func TestElCobroPorReasignacionImpagoEntraAlBarridoYPagarloLoSaca(t *testing.T) {
	store, pool := setup(t)
	reassignments := persistence.NewReassignmentRepository(pool)
	balances := persistence.NewBalanceRepository(pool)
	customer := newCustomer(t, pool)
	order := placedOrder(t, store, pool, customer)
	setFee(t, reassignments, "2.50")
	_, err := reassignments.ApplyReassignmentCharge(context.Background(), order, buildReassignment(order))
	require.NoError(t, err)

	obligations, err := balances.ListActiveObligations(context.Background())
	require.NoError(t, err)
	mine := 0
	for _, o := range obligations {
		if o.CustomerID == customer {
			mine++
		}
	}
	assert.Equal(t, 1, mine)

	_, err = balances.SettleBalance(context.Background(), customer, decimal.RequireFromString("2.50"), time.Now())
	require.NoError(t, err)

	assert.True(t, balanceOf(t, pool, customer).IsZero())
	assert.Equal(t, 0, countRows(t, pool, `SELECT count(*) FROM reassignment_charges WHERE customer_id = $1 AND paid_at IS NULL`, customer))
}

// The customer's breakdown must add up to the total it owes: the reassignment charge is a line like any other.
func TestElCobroPorReasignacionImpagoApareceComoLineaDelSaldoYDejaDeAparecerAlPagar(t *testing.T) {
	store, pool := setup(t)
	reassignments := persistence.NewReassignmentRepository(pool)
	balances := persistence.NewBalanceRepository(pool)
	customer := newCustomer(t, pool)
	order := placedOrder(t, store, pool, customer)
	setFee(t, reassignments, "2.50")
	_, err := reassignments.ApplyReassignmentCharge(context.Background(), order, buildReassignment(order))
	require.NoError(t, err)

	lines, err := balances.ListOwedLines(context.Background(), customer)

	require.NoError(t, err)
	require.Len(t, lines, 1)
	assert.Equal(t, domain.BalanceLineReassignment, lines[0].Kind)
	assert.Equal(t, order, lines[0].OrderID)
	assert.Equal(t, "REASSIGNMENT", lines[0].Reason)
	assert.True(t, lines[0].Amount.Equal(decimal.RequireFromString("2.50")))

	_, err = balances.SettleBalance(context.Background(), customer, decimal.RequireFromString("2.50"), time.Now())
	require.NoError(t, err)
	lines, err = balances.ListOwedLines(context.Background(), customer)
	require.NoError(t, err)
	assert.Empty(t, lines)
}
