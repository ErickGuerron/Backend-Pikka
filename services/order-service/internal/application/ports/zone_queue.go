package ports

import (
	"context"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// ZoneCapacity reads the daily and route limits from the Zone Service.
type ZoneCapacity interface {
	ZoneLimits(ctx context.Context, zoneID string) (domain.ZoneLimits, error)
}

// ZoneQueueRepository is the persistence side of the zone waiting line.
type ZoneQueueRepository interface {
	FillZoneQueue(ctx context.Context, zoneID, deliveryDate string, limits domain.ZoneLimits) (int, error)
	QueuePosition(ctx context.Context, order domain.Order) (int, error)
	WaitingZoneDays(ctx context.Context) ([]domain.ZoneDay, error)
	ZoneActiveCount(ctx context.Context, zoneID, deliveryDate string) (int, error)
}
