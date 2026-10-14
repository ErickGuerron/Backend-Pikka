//go:build integration

package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/infrastructure/postgres"
)

func openRecorder(t *testing.T) (*postgres.DeliveryRecorder, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("DRIVER_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("DRIVER_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	_, err = pool.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS drivers`)
	require.NoError(t, err)
	require.NoError(t, postgres.Migrate(dsn))
	_, err = pool.Exec(ctx, `TRUNCATE deliveries`)
	require.NoError(t, err)
	return postgres.NewDeliveryRecorder(pool), pool
}

func TestDeliveryRecorderGuardaYRechazaDuplicado(t *testing.T) {
	rec, pool := openRecorder(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 13, 9, 30, 0, 0, time.UTC)
	entrega, err := domain.NewDeliveryRecord("stop-1", domain.OutcomeNotDelivered, domain.ReasonClientAbsent, at,
		domain.GeoPoint{Latitude: -0.1810, Longitude: -78.4680}, "")
	require.NoError(t, err)
	arrival := domain.ArrivalCheck{WithinRadius: true, WithinWindow: true, DistanceM: 40}

	require.NoError(t, rec.Record(ctx, "route-1", entrega, arrival))
	err = rec.Record(ctx, "route-1", entrega, arrival)

	assert.ErrorIs(t, err, ports.ErrDeliveryAlreadyRecorded)
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM deliveries WHERE route_id = 'route-1'`).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestDeliveryRecorderRechazaMotivoInconsistente(t *testing.T) {
	rec, _ := openRecorder(t)
	at := time.Date(2026, 10, 13, 9, 30, 0, 0, time.UTC)
	loc := domain.GeoPoint{Latitude: -0.1810, Longitude: -78.4680}
	entrega, err := domain.NewDeliveryRecord("stop-2", domain.OutcomeDelivered, "", at, loc, "")
	require.NoError(t, err)
	entrega.Reason = domain.ReasonClientAbsent // un DELIVERED con motivo no debe llegar a la base

	err = rec.Record(context.Background(), "route-1", entrega, domain.ArrivalCheck{})

	assert.Error(t, err, "la restricción de la base debe rechazarlo")
}
