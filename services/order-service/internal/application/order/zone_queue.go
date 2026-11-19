package order

import (
	"context"
	"log/slog"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// ZoneQueue keeps the zone waiting line moving. An order that finds its zone without a free
// route slot stays PENDING, shown to the customer as "en espera". Every event that frees a
// slot (a stop closed, an order cancelled, routes assigned) fills the oldest waiting orders
// into the routes that have room, so the customer is assigned to a driver of its own zone.
type ZoneQueue struct {
	repo     ports.ZoneQueueRepository
	capacity ports.RouteCapacity
}

// NewZoneQueue wires the queue to its persistence and to the zone capacity source.
func NewZoneQueue(repo ports.ZoneQueueRepository, capacity ports.RouteCapacity) *ZoneQueue {
	return &ZoneQueue{repo: repo, capacity: capacity}
}

// Fill routes the oldest waiting orders of a zone and delivery date into routes with room and
// returns how many it routed. An order without zone or date has nothing to route.
func (q *ZoneQueue) Fill(ctx context.Context, zoneID, deliveryDate string) (int, error) {
	if zoneID == "" || deliveryDate == "" {
		return 0, nil
	}
	maxPerRoute, err := q.capacity.MaxOrdersPerRoute(ctx, zoneID)
	if err != nil {
		return 0, err
	}
	if maxPerRoute <= 0 {
		return 0, nil
	}
	return q.repo.FillZoneQueue(ctx, zoneID, deliveryDate, maxPerRoute)
}

// Position returns the customer's place in line, or 0 when the order is not waiting.
func (q *ZoneQueue) Position(ctx context.Context, order domain.Order) (int, error) {
	if order.Status != domain.StatusPending || order.RouteID != "" || order.ZoneID == "" {
		return 0, nil
	}
	return q.repo.QueuePosition(ctx, order)
}

// positionOf is nil-safe: without a queue wired, no order is waiting.
func (q *ZoneQueue) positionOf(ctx context.Context, order domain.Order) (int, error) {
	if q == nil {
		return 0, nil
	}
	return q.Position(ctx, order)
}

// fillNonFatal runs after the event is already committed. The order or the stop closure is saved
// either way, so a failed fill is logged and the next freeing event tries again.
func (q *ZoneQueue) fillNonFatal(ctx context.Context, zoneID, deliveryDate string) {
	if q == nil {
		return
	}
	if _, err := q.Fill(ctx, zoneID, deliveryDate); err != nil {
		slog.Default().Warn("zone queue fill failed; the next freeing event retries", "zone", zoneID, "error", err)
	}
}
