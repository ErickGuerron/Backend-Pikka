//go:build integration

package grpc_test

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/driver"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/infrastructure/postgres"
)

const integrationDriverID = "11111111-1111-4111-8111-111111111111"

// La reserva concurrente por gRPC contra PostgreSQL real: el contrato debe dar una sola ganadora.
func TestReserveDriverGRPCConcurrentePostgres(t *testing.T) {
	dsn := os.Getenv("DRIVER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("DRIVER_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	require.NoError(t, postgres.Migrate(dsn))
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	_, err = pool.Exec(ctx, `TRUNCATE drivers`)
	require.NoError(t, err)

	repo := postgres.NewDriverRepository(pool)
	d, err := domain.NewDriver(integrationDriverID, domain.NewDriverInput{
		UserID: "22222222-2222-4222-8222-222222222222", FullName: "Ana Ruiz", Phone: "0991234567",
	}, time.Now())
	require.NoError(t, err)
	require.NoError(t, repo.Create(ctx, d))

	client := serve(t, driver.NewService(repo, time.Now))

	const contenders = 20
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		winners  int
		codesGot = map[codes.Code]int{}
	)
	for i := 0; i < contenders; i++ {
		wg.Add(1)
		go func(routeID string) {
			defer wg.Done()
			_, err := client.ReserveDriver(ctx, &driversv1.ReserveDriverRequest{DriverId: integrationDriverID, RouteId: routeID})
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				winners++
				return
			}
			codesGot[status.Code(err)]++
		}(fmt.Sprintf("route-%d", i))
	}
	wg.Wait()

	assert.Equal(t, 1, winners, "exactamente una ruta reserva al repartidor")
	// Los perdedores ven FAILED_PRECONDITION (ya reservado) o ABORTED si agotaron los reintentos
	// de versión; ABORTED pide al cliente reintentar, y no hay otro código válido aquí.
	for code, n := range codesGot {
		assert.Contains(t, []codes.Code{codes.FailedPrecondition, codes.Aborted}, code, "código inesperado con %d respuestas", n)
	}

	stored, err := repo.Get(ctx, integrationDriverID)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusAssigned, stored.Status)
	assert.NotEmpty(t, stored.RouteID)
}
