package driver_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/adapters/memory"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/driver"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

// Decisión D1: una entrega exitosa fuera del radio se bloquea. Un no entregado se registra siempre.

func TestRegisterDeliveryDeliveredOutsideRadiusIsBlocked(t *testing.T) {
	repo := memory.NewDriverRepository()
	recorder := memory.NewDeliveryRecorder()
	onRoute(t, repo, "d1", "route-1")
	uc := driver.NewDeliveries(repo, recorder, 50)

	_, err := uc.RegisterDelivery(context.Background(), "d1", "route-1", stop, deliveryInput("stop-1", farStop))

	assert.ErrorIs(t, err, domain.ErrOutsideRadius)
	assert.Equal(t, 0, recorder.Count(), "a blocked delivery is not recorded")
}

func TestRegisterDeliveryDeliveredInsideRadiusIsAccepted(t *testing.T) {
	repo := memory.NewDriverRepository()
	onRoute(t, repo, "d1", "route-1")
	uc := driver.NewDeliveries(repo, memory.NewDeliveryRecorder(), 50)

	_, err := uc.RegisterDelivery(context.Background(), "d1", "route-1", stop, deliveryInput("stop-1", nearStop))

	require.NoError(t, err)
}

func TestRegisterDeliveryNotDeliveredAtDistanceIsRecorded(t *testing.T) {
	repo := memory.NewDriverRepository()
	recorder := memory.NewDeliveryRecorder()
	onRoute(t, repo, "d1", "route-1")
	uc := driver.NewDeliveries(repo, recorder, 50)
	in := driver.DeliveryInput{
		StopID: "stop-1", Outcome: domain.OutcomeNotDelivered, Reason: domain.ReasonWrongAddress,
		At: t0.Add(1), Location: farStop,
	}

	_, err := uc.RegisterDelivery(context.Background(), "d1", "route-1", stop, in)

	require.NoError(t, err)
	assert.Equal(t, 1, recorder.Count())
}
