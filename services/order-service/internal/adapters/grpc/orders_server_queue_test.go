package grpc

import (
	"testing"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// The customer reads its place in line through the proto Order; the Gateway turns it into queuePosition.
func TestOrderToProtoCarriesQueuePosition(t *testing.T) {
	got := orderToProto(domain.Order{ID: uuid.NewString(), Status: domain.StatusPending, QueuePosition: 3})
	if got.GetQueuePosition() != 3 {
		t.Fatalf("queue position = %d, want 3", got.GetQueuePosition())
	}
}
