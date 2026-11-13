package http

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
)

func TestReorderETAAccountsForWaitingAndServiceTime(t *testing.T) {
	timezone := time.FixedZone("ECT", -5*3600)
	now := time.Date(2026, 11, 13, 15, 0, 0, 0, time.UTC)
	early := &ordersv1.DriverRouteStop{OrderId: "a", DeliveryDate: "2026-11-13", WindowStart: "10:00", WindowEnd: "10:15"}
	late := &ordersv1.DriverRouteStop{OrderId: "b", DeliveryDate: "2026-11-13", WindowStart: "10:10", WindowEnd: "11:00"}

	before, err := windowFeasibility(now, timezone, []*ordersv1.DriverRouteStop{early, late}, []float64{60, 60})
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"a": true, "b": true}, before)

	after, err := windowFeasibility(now, timezone, []*ordersv1.DriverRouteStop{late, early}, []float64{60, 60})
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"a": false, "b": true}, after)
}
