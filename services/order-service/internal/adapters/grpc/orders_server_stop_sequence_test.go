package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// Staff reads the planned stop place from Order, so it must travel in the proto.
func TestOrderToProto_CarriesTheStopSequence(t *testing.T) {
	assert.Equal(t, int32(3), orderToProto(domain.Order{RouteID: "r-1", StopSequence: 3}).GetStopSequence())
	assert.Zero(t, orderToProto(domain.Order{}).GetStopSequence(), "an order without a route has no stop place")
}
