package driver_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/adapters/memory"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/application/driver"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

// Plan (BOLA): a driver on route-2 cannot register a stop of route-1, which belongs to driver d1.
func TestPlan_RegisterDelivery_StopOfAnotherDriversRoute_RejectedAndNothingRecorded(t *testing.T) {
	repo := memory.NewDriverRepository()
	recorder := memory.NewDeliveryRecorder()
	onRoute(t, repo, "d1", "route-1")
	onRoute(t, repo, "d2", "route-2")
	uc := driver.NewDeliveries(repo, recorder, 50)

	_, err := uc.RegisterDelivery(context.Background(), "d2", "route-1", stop, deliveryInput("stop-1", nearStop))

	assert.ErrorIs(t, err, domain.ErrRouteMismatch)
	assert.Zero(t, recorder.Count())
}
