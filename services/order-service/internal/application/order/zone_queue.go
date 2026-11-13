package order

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// ZoneQueue admits the oldest pending orders into reserved routes when both limits
// allow it. Order events and the periodic sweep use the same fill operation.
type ZoneQueue struct {
	repo     ports.ZoneQueueRepository
	capacity ports.ZoneCapacity
}

// NewZoneQueue wires the queue to its persistence and to the zone capacity source.
func NewZoneQueue(repo ports.ZoneQueueRepository, capacity ports.ZoneCapacity) *ZoneQueue {
	return &ZoneQueue{repo: repo, capacity: capacity}
}

// Fill routes the oldest waiting orders of a zone and delivery date into routes with room and
// returns how many it routed. An order without zone or date has nothing to route.
func (q *ZoneQueue) Fill(ctx context.Context, zoneID, deliveryDate string) (int, error) {
	if zoneID == "" || deliveryDate == "" {
		return 0, nil
	}
	limits, err := q.capacity.ZoneLimits(ctx, zoneID)
	if err != nil {
		return 0, err
	}
	if limits.PerRoute <= 0 || limits.Daily <= 0 {
		return 0, nil
	}
	return q.repo.FillZoneQueue(ctx, zoneID, deliveryDate, limits)
}

// Position returns the customer's place in line, or 0 when the order is not waiting.
func (q *ZoneQueue) Position(ctx context.Context, order domain.Order) (int, error) {
	if order.Status != domain.StatusPending || order.RouteID != "" || order.ZoneID == "" {
		return 0, nil
	}
	position, err := q.repo.QueuePosition(ctx, order)
	if err != nil || position == 0 {
		return position, err
	}
	_, remaining, err := q.capacityForDay(ctx, order.ZoneID, order.DeliveryDate.String())
	if err != nil {
		// Zone Service can be unavailable independently of the order read. Keep
		// the order visible and omit the capacity notice until the limit is known.
		slog.Default().Warn("zone queue capacity unavailable", "zone", order.ZoneID, "error", err)
		return 0, nil
	}
	return max(0, position-remaining), nil
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

// Sweep retries every waiting zone and day even if one zone is temporarily unavailable.
func (q *ZoneQueue) Sweep(ctx context.Context) (int, error) {
	days, err := q.repo.WaitingZoneDays(ctx)
	if err != nil {
		return 0, err
	}
	var total int
	var failures []error
	for _, day := range days {
		if ctx.Err() != nil {
			return total, errors.Join(append(failures, ctx.Err())...)
		}
		count, err := q.Fill(ctx, day.ZoneID, day.DeliveryDate)
		total += count
		if err != nil {
			failures = append(failures, fmt.Errorf("zone %s on %s: %w", day.ZoneID, day.DeliveryDate, err))
		}
	}
	return total, errors.Join(failures...)
}

// Occupancy returns the active orders and both limits for a zone and day.
func (q *ZoneQueue) Occupancy(ctx context.Context, zoneID, date string) (int, domain.ZoneLimits, error) {
	limits, err := q.capacity.ZoneLimits(ctx, zoneID)
	if err != nil {
		return 0, domain.ZoneLimits{}, err
	}
	active, err := q.repo.ZoneActiveCount(ctx, zoneID, date)
	return active, limits, err
}

func (q *ZoneQueue) capacityForDay(ctx context.Context, zoneID, date string) (domain.ZoneLimits, int, error) {
	active, limits, err := q.Occupancy(ctx, zoneID, date)
	return limits, max(0, limits.Daily-active), err
}
