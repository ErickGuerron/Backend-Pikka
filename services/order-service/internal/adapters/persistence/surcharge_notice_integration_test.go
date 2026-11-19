//go:build integration

package persistence_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/adapters/persistence"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// realOrder crea un pedido de verdad: el aviso del recargo lo referencia con una clave foranea, asi que un UUID al azar
// ya no sirve como pedido no entregado.
func realOrder(t *testing.T, store *persistence.Store, pool *pgxpool.Pool, customer string) string {
	t.Helper()
	p := newProduct(t, store, "Manzana "+uuid.NewString()[:8], stock(50))
	placed, err := store.PlaceOrder(context.Background(), newOrder(t, customer), []domain.OrderLine{lineFor(t, p, 1)}, nil, nil)
	require.NoError(t, err)
	return placed.ID
}

func noticesOf(t *testing.T, pool *pgxpool.Pool, customer string) []domain.Notification {
	t.Helper()
	list, err := persistence.NewNotificationRepository(pool).ListByCustomer(context.Background(), uuid.MustParse(customer))
	require.NoError(t, err)
	return list
}

func noticesOfKind(t *testing.T, pool *pgxpool.Pool, customer string, kind domain.NotificationKind) []domain.Notification {
	t.Helper()
	var out []domain.Notification
	for _, n := range noticesOf(t, pool, customer) {
		if n.Kind == kind {
			out = append(out, n)
		}
	}
	return out
}

// ADR 0006 seccion 5: el reloj de 2 semanas empieza cuando el sistema confirma la penalizacion y notifica al cliente.
func TestUnRecargoNuevoConMontoDejaUnAvisoConMotivoMontoYFechaLimite(t *testing.T) {
	store, pool := setup(t)
	repo := persistence.NewSurchargeRepository(pool)
	customer := newCustomer(t, pool)
	order := realOrder(t, store, pool, customer)

	saved, created, err := repo.RecordSurchargeIncident(context.Background(), pricedIncident(t, customer, order, "CLIENT_REFUSED", "20.00"))

	require.NoError(t, err)
	require.True(t, created)
	notices := noticesOfKind(t, pool, customer, domain.NotificationSurchargeApplied)
	require.Len(t, notices, 1)
	assert.Equal(t, order, notices[0].OrderID)
	assert.False(t, notices[0].Read)
	assert.Equal(t, "CLIENT_REFUSED,3.00,"+saved.DueAt().UTC().Format(time.RFC3339), notices[0].Detail)
}

func TestRepetirLaIncidenciaNoDejaUnSegundoAviso(t *testing.T) {
	store, pool := setup(t)
	repo := persistence.NewSurchargeRepository(pool)
	customer := newCustomer(t, pool)
	order := realOrder(t, store, pool, customer)
	ctx := context.Background()

	_, _, err := repo.RecordSurchargeIncident(ctx, pricedIncident(t, customer, order, "CLIENT_ABSENT", "20.00"))
	require.NoError(t, err)
	_, created, err := repo.RecordSurchargeIncident(ctx, pricedIncident(t, customer, order, "CLIENT_ABSENT", "20.00"))

	require.NoError(t, err)
	assert.False(t, created)
	assert.Len(t, noticesOfKind(t, pool, customer, domain.NotificationSurchargeApplied), 1)
}

func TestIncidenciaConcurrenteDejaUnSoloAviso(t *testing.T) {
	store, pool := setup(t)
	repo := persistence.NewSurchargeRepository(pool)
	customer := newCustomer(t, pool)
	order := realOrder(t, store, pool, customer)

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := repo.RecordSurchargeIncident(context.Background(), pricedIncident(t, customer, order, "CLIENT_ABSENT", "20.00"))
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	assert.Len(t, noticesOfKind(t, pool, customer, domain.NotificationSurchargeApplied), 1)
}

// Sin monto no hay nada que pagar, y una direccion incorrecta espera al operador: ninguna se notifica como definitiva.
func TestNoSeAvisaLoQueAunNoSeDebe(t *testing.T) {
	store, pool := setup(t)
	repo := persistence.NewSurchargeRepository(pool)
	unpriced, waiting := newCustomer(t, pool), newCustomer(t, pool)
	ctx := context.Background()

	_, _, err := repo.RecordSurchargeIncident(ctx, newIncident(t, unpriced, realOrder(t, store, pool, unpriced)))
	require.NoError(t, err)
	_, _, err = repo.RecordSurchargeIncident(ctx, pricedIncident(t, waiting, realOrder(t, store, pool, waiting), "WRONG_ADDRESS", "10.00"))
	require.NoError(t, err)

	assert.Empty(t, noticesOf(t, pool, unpriced))
	assert.Empty(t, noticesOf(t, pool, waiting))
}

func TestConfirmarUnaDireccionIncorrectaAvisaConLaFechaDelOperadorYDispensarNo(t *testing.T) {
	store, pool := setup(t)
	repo := persistence.NewSurchargeRepository(pool)
	confirmed, waived := newCustomer(t, pool), newCustomer(t, pool)
	ctx := context.Background()
	decide := func(d domain.SurchargeDecision, at time.Time) func(domain.Surcharge) (domain.Surcharge, error) {
		return func(s domain.Surcharge) (domain.Surcharge, error) {
			return s.Resolve(d, uuid.NewString(), "Verificado con el cliente", at)
		}
	}
	var ids [2]string
	for i, c := range []string{confirmed, waived} {
		saved, _, err := repo.RecordSurchargeIncident(ctx, pricedIncident(t, c, realOrder(t, store, pool, c), "WRONG_ADDRESS", "10.00"))
		require.NoError(t, err)
		ids[i] = saved.ID
	}
	confirmedAt := time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)

	_, err := repo.ResolveSurchargeVerification(ctx, ids[0], decide(domain.DecisionConfirm, confirmedAt))
	require.NoError(t, err)
	_, err = repo.ResolveSurchargeVerification(ctx, ids[1], decide(domain.DecisionWaive, confirmedAt))
	require.NoError(t, err)

	notices := noticesOfKind(t, pool, confirmed, domain.NotificationSurchargeApplied)
	require.Len(t, notices, 1)
	assert.Equal(t, "WRONG_ADDRESS,1.50,"+confirmedAt.AddDate(0, 0, 14).Format(time.RFC3339), notices[0].Detail,
		"el plazo corre desde que el operador confirma, no desde la no entrega")
	assert.Empty(t, noticesOf(t, pool, waived))
}

// El aviso de bloqueo es de la cuenta: sin pedido, sin detalle, una sola vez por bloqueo.
func TestBloquearPorSaldoAvisaUnaVezACadaCuentaNuevamenteBloqueada(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewBalanceRepository(pool)
	active, again, byIncidents := newCustomer(t, pool), newCustomer(t, pool), newCustomer(t, pool)
	setStatus(t, pool, again, "BLOQUEADO_POR_SALDO")
	setStatus(t, pool, byIncidents, "BLOQUEADO_POR_INCIDENCIAS")
	ctx := context.Background()

	n, err := repo.BlockCustomersForBalance(ctx, []string{active, again, byIncidents})
	require.NoError(t, err)
	require.Equal(t, 1, n)

	blocked := noticesOfKind(t, pool, active, domain.NotificationAccountBlocked)
	require.Len(t, blocked, 1)
	assert.Empty(t, blocked[0].OrderID)
	assert.Empty(t, blocked[0].Detail)
	assert.Empty(t, noticesOf(t, pool, again), "ya estaba bloqueada: no se vuelve a avisar")
	assert.Empty(t, noticesOf(t, pool, byIncidents), "un bloqueo por incidencias no se avisa como saldo")

	n, err = repo.BlockCustomersForBalance(ctx, []string{active})
	require.NoError(t, err)
	assert.Zero(t, n)
	assert.Len(t, noticesOfKind(t, pool, active, domain.NotificationAccountBlocked), 1, "repetir el barrido no avisa otra vez")
}

// De punta a punta con el barrido real: vencido -> bloqueado -> un aviso.
func TestElBarridoDeSaldoVencidoDejaElAvisoDeBloqueo(t *testing.T) {
	_, pool := setup(t)
	repo := persistence.NewBalanceRepository(pool)
	customer := newCustomer(t, pool)
	insertAt(t, pool, customer, domain.SurchargePending, time.Now().AddDate(0, 0, -20))
	setBalance(t, pool, customer, "3.00")
	sweep := orderapp.NewBlockOverdueBalances(repo)

	_, err := sweep.Run(context.Background())
	require.NoError(t, err)
	_, err = sweep.Run(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "BLOQUEADO_POR_SALDO", statusOf(t, pool, customer))
	assert.Len(t, noticesOfKind(t, pool, customer, domain.NotificationAccountBlocked), 1)
}

// La base protege la regla: solo el aviso de la cuenta puede no tener pedido, y solo valen los tipos conocidos.
func TestLaBaseExigePedidoEnLosAvisosDePedidoYRechazaTiposDesconocidos(t *testing.T) {
	_, pool := setup(t)
	customer := newCustomer(t, pool)
	insert := func(kind string, order any) error {
		_, err := pool.Exec(context.Background(),
			`INSERT INTO customer_notifications (customer_id, order_id, kind, detail) VALUES ($1, $2, $3, '')`, customer, order, kind)
		return err
	}

	assert.NoError(t, insert("ACCOUNT_BLOCKED_BY_BALANCE", nil))
	assert.Error(t, insert("SURCHARGE_APPLIED", nil), "un aviso de recargo sin pedido")
	assert.Error(t, insert("ORDER_REJECTED", nil), "un aviso de pedido sin pedido")
	assert.Error(t, insert("TIPO_INVENTADO", nil))
}
