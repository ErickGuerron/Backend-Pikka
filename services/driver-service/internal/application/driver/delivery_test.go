package driver_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/adapters/memory"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/driver"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain/domaintest"
)

var (
	stopPoint = domain.GeoPoint{Latitude: -0.1807, Longitude: -78.4678}
	nearStop  = domain.GeoPoint{Latitude: -0.1810, Longitude: -78.4680} // ~40 m
	farStop   = domain.GeoPoint{Latitude: -0.2500, Longitude: -78.5000}
	stop      = domain.Stop{Location: stopPoint, Window: domain.ArrivalWindow{Start: t0, End: t0.Add(3 * time.Hour)}}
)

// onRoute deja al repartidor ON_ROUTE en la ruta routeID dentro del repositorio.
func onRoute(t *testing.T, repo *memory.DriverRepository, id, routeID string) {
	t.Helper()
	ctx := context.Background()
	d, err := domain.NewDriver(id, domaintest.Input("user-"+id), t0)
	require.NoError(t, err)
	require.NoError(t, repo.Create(ctx, d))
	stored, err := repo.Get(ctx, id)
	require.NoError(t, err)
	require.NoError(t, stored.Reserve(routeID, t0))
	require.NoError(t, stored.StartRoute(routeID, t0))
	require.NoError(t, repo.Save(ctx, stored, stored.Version-2))
}

func deliveryInput(stopID string, loc domain.GeoPoint) driver.DeliveryInput {
	return driver.DeliveryInput{StopID: stopID, Outcome: domain.OutcomeDelivered, At: t0.Add(time.Hour), Location: loc}
}

func TestRegisterDeliveryEntregaValida(t *testing.T) {
	repo := memory.NewDriverRepository()
	recorder := memory.NewDeliveryRecorder()
	onRoute(t, repo, "d1", "route-1")
	uc := driver.NewDeliveries(repo, recorder, 50)

	got, err := uc.RegisterDelivery(context.Background(), "d1", "route-1", stop, deliveryInput("stop-1", nearStop))

	require.NoError(t, err)
	assert.Equal(t, domain.OutcomeDelivered, got.Record.Outcome)
	assert.True(t, got.Arrival.Valid())
	assert.Empty(t, got.Record.Reason)
}

func TestRegisterDeliveryNoEntregaLejosNoSeRechaza(t *testing.T) {
	repo := memory.NewDriverRepository()
	onRoute(t, repo, "d1", "route-1")
	uc := driver.NewDeliveries(repo, memory.NewDeliveryRecorder(), 50)
	in := driver.DeliveryInput{
		StopID: "stop-1", Outcome: domain.OutcomeNotDelivered, Reason: domain.ReasonClientAbsent,
		At: t0.Add(time.Hour), Location: farStop,
	}

	got, err := uc.RegisterDelivery(context.Background(), "d1", "route-1", stop, in)

	require.NoError(t, err, "el registro se guarda; la validez de la llegada solo informa la penalización")
	assert.False(t, got.Arrival.WithinRadius)
	assert.False(t, got.Arrival.Valid())
}

func TestRegisterDeliveryUsaElRadioConfigurado(t *testing.T) {
	repo := memory.NewDriverRepository()
	onRoute(t, repo, "d1", "route-1")
	uc := driver.NewDeliveries(repo, memory.NewDeliveryRecorder(), 10)

	got, err := uc.RegisterDelivery(context.Background(), "d1", "route-1", stop, deliveryInput("stop-1", nearStop))

	require.NoError(t, err)
	assert.False(t, got.Arrival.WithinRadius, "~40 m no entran en un radio de 10 m")
}

func TestRegisterDeliveryRequiereEstarEnRuta(t *testing.T) {
	repo := memory.NewDriverRepository()
	d, err := domain.NewDriver("d1", domaintest.Input("u"), t0)
	require.NoError(t, err)
	require.NoError(t, repo.Create(context.Background(), d))
	uc := driver.NewDeliveries(repo, memory.NewDeliveryRecorder(), 50)

	_, err = uc.RegisterDelivery(context.Background(), "d1", "route-1", stop, deliveryInput("stop-1", nearStop))

	assert.ErrorIs(t, err, domain.ErrInvalidTransition)
}

func TestRegisterDeliveryRutaDistinta(t *testing.T) {
	repo := memory.NewDriverRepository()
	onRoute(t, repo, "d1", "route-1")
	uc := driver.NewDeliveries(repo, memory.NewDeliveryRecorder(), 50)

	_, err := uc.RegisterDelivery(context.Background(), "d1", "route-2", stop, deliveryInput("stop-1", nearStop))

	assert.ErrorIs(t, err, domain.ErrRouteMismatch)
}

func TestRegisterDeliveryParadaDuplicada(t *testing.T) {
	repo := memory.NewDriverRepository()
	onRoute(t, repo, "d1", "route-1")
	uc := driver.NewDeliveries(repo, memory.NewDeliveryRecorder(), 50)
	ctx := context.Background()
	_, err := uc.RegisterDelivery(ctx, "d1", "route-1", stop, deliveryInput("stop-1", nearStop))
	require.NoError(t, err)

	_, err = uc.RegisterDelivery(ctx, "d1", "route-1", stop, deliveryInput("stop-1", nearStop))

	assert.ErrorIs(t, err, ports.ErrDeliveryAlreadyRecorded)
}

func TestRegisterDeliveryValidaElRegistro(t *testing.T) {
	repo := memory.NewDriverRepository()
	onRoute(t, repo, "d1", "route-1")
	uc := driver.NewDeliveries(repo, memory.NewDeliveryRecorder(), 50)
	in := driver.DeliveryInput{StopID: "stop-1", Outcome: domain.OutcomeNotDelivered, At: t0, Location: nearStop}

	_, err := uc.RegisterDelivery(context.Background(), "d1", "route-1", stop, in)

	assert.ErrorIs(t, err, domain.ErrReasonRequired)
}

func TestRegisterDeliveryRepartidorInexistente(t *testing.T) {
	uc := driver.NewDeliveries(memory.NewDriverRepository(), memory.NewDeliveryRecorder(), 50)

	_, err := uc.RegisterDelivery(context.Background(), "nadie", "route-1", stop, deliveryInput("stop-1", nearStop))

	assert.ErrorIs(t, err, ports.ErrDriverNotFound)
}
