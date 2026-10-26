package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// FillZoneQueue routes the oldest waiting orders of one zone and day into the driver routes of that
// zone that still have room. Each routed order takes the next stop sequence of its route.
//
// The whole fill runs in one transaction under an advisory lock keyed by zone and day, so two events
// that free slots at the same time cannot route the same order or overfill a route. Fills of other
// zones or days, and every other order operation, keep running in parallel.
func (r *OrderRepository) FillZoneQueue(ctx context.Context, zoneID, deliveryDate string, maxPerRoute int) (int, error) {
	if maxPerRoute <= 0 {
		return 0, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("zone queue: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, zoneID+"|"+deliveryDate); err != nil {
		return 0, fmt.Errorf("zone queue: lock: %w", err)
	}

	routed := 0
	for {
		routeID, sequence, found, err := routeWithRoom(ctx, tx, zoneID, deliveryDate, maxPerRoute)
		if err != nil {
			return 0, err
		}
		if !found {
			break
		}
		waiting, found, err := oldestWaiting(ctx, tx, zoneID, deliveryDate)
		if err != nil {
			return 0, err
		}
		if !found {
			break
		}
		assigned, err := waiting.AssignToRoute(routeID, sequence)
		if err != nil {
			return 0, fmt.Errorf("zone queue: assign order %s: %w", waiting.ID, err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE orders
			    SET status = $2, route_id = $3, stop_sequence = $4, version = version + 1, updated_at = now()
			  WHERE id = $1`,
			assigned.ID, string(assigned.Status), assigned.RouteID, assigned.StopSequence,
		); err != nil {
			if isDuplicateStopSequence(err) {
				return 0, domain.ErrDuplicateStopSequence
			}
			return 0, fmt.Errorf("zone queue: update order %s: %w", assigned.ID, err)
		}
		routed++
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("zone queue: commit: %w", err)
	}
	return routed, nil
}

// QueuePosition counts the waiting orders of the same zone and day that were registered up to and
// including this one. Only a PENDING order without route is in line.
func (r *OrderRepository) QueuePosition(ctx context.Context, order domain.Order) (int, error) {
	var position int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM orders
		  WHERE zone_id = $1::uuid AND delivery_date = $2::date
		    AND status = 'PENDING' AND route_id IS NULL
		    AND (created_at, id) <= ($3::timestamptz, $4::uuid)`,
		order.ZoneID, order.DeliveryDate.String(), order.CreatedAt, order.ID,
	).Scan(&position)
	if err != nil {
		return 0, fmt.Errorf("zone queue: position: %w", err)
	}
	return position, nil
}

// routeWithRoom returns the route of the zone and day with the fewest active orders, with the next stop
// sequence it would use, as long as the zone as a whole still has room. The capacity is the zone's total
// of active orders per day, shared by all its routes. Closed orders keep their sequence, so the next
// sequence comes from the highest one ever used on the route.
func routeWithRoom(ctx context.Context, tx pgx.Tx, zoneID, deliveryDate string, maxPerZone int) (string, int, bool, error) {
	var zoneActive int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM orders
		  WHERE zone_id = $1::uuid AND delivery_date = $2::date
		    AND status IN ('ROUTE_ASSIGNED', 'PICKED_UP', 'IN_TRANSIT')`,
		zoneID, deliveryDate,
	).Scan(&zoneActive); err != nil {
		return "", 0, false, fmt.Errorf("zone queue: zone active: %w", err)
	}
	if zoneActive >= maxPerZone {
		return "", 0, false, nil
	}

	var routeID string
	var lastSequence, active int
	err := tx.QueryRow(ctx,
		`SELECT route_id,
		        COALESCE(max(stop_sequence), 0),
		        count(*) FILTER (WHERE status IN ('ROUTE_ASSIGNED', 'PICKED_UP', 'IN_TRANSIT')) AS active
		   FROM orders
		  WHERE zone_id = $1::uuid AND delivery_date = $2::date AND route_id IS NOT NULL
		  GROUP BY route_id
		  ORDER BY active ASC, route_id ASC
		  LIMIT 1`,
		zoneID, deliveryDate,
	).Scan(&routeID, &lastSequence, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, fmt.Errorf("zone queue: route with room: %w", err)
	}
	return routeID, lastSequence + 1, true, nil
}

// oldestWaiting locks the oldest PENDING order of the zone and day that has no route. Rows locked by
// another transaction, such as a customer cancelling, are skipped rather than waited for.
func oldestWaiting(ctx context.Context, tx pgx.Tx, zoneID, deliveryDate string) (domain.Order, bool, error) {
	order, err := scanOrder(tx.QueryRow(ctx,
		`SELECT `+orderColumns+` FROM orders
		  WHERE zone_id = $1::uuid AND delivery_date = $2::date
		    AND status = 'PENDING' AND route_id IS NULL
		  ORDER BY created_at ASC, id ASC
		  LIMIT 1
		  FOR UPDATE SKIP LOCKED`,
		zoneID, deliveryDate,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, false, nil
	}
	if err != nil {
		return domain.Order{}, false, fmt.Errorf("zone queue: oldest waiting: %w", err)
	}
	return order, true, nil
}
