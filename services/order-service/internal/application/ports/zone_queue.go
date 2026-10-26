package ports

import (
	"context"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// RouteCapacity answers how many orders fit in one driver route of a zone (Zone Service).
// Zero means the zone has no capacity to offer, so no waiting order can be routed there.
type RouteCapacity interface {
	MaxOrdersPerRoute(ctx context.Context, zoneID string) (int, error)
}

// ZoneQueueRepository is the persistence side of the zone waiting line. Implementations
// serialize the work per zone and delivery date so two events cannot route the same order.
type ZoneQueueRepository interface {
	// FillZoneQueue routes the oldest waiting orders of the zone and day into driver routes that
	// still have room, one at a time, until no route has room or no order waits. It returns how
	// many orders it routed. maxPerRoute 0 routes nothing.
	FillZoneQueue(ctx context.Context, zoneID, deliveryDate string, maxPerRoute int) (int, error)
	// QueuePosition returns the 1-based FIFO place of a waiting order, or 0 when the order is not
	// waiting (not PENDING, already routed, or without zone).
	QueuePosition(ctx context.Context, order domain.Order) (int, error)
}
