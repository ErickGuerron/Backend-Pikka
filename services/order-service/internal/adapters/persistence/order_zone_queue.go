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
func (r *OrderRepository) FillZoneQueue(ctx context.Context, zoneID, deliveryDate string, limits domain.ZoneLimits) (int, error) {
	if limits.Daily <= 0 || limits.PerRoute <= 0 {
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
	var unavailableRoutes []string
	verified := make(map[string]bool)
	for {
		routeID, sequence, found, err := routeWithRoom(ctx, tx, zoneID, deliveryDate, limits, unavailableRoutes)
		if err != nil {
			return 0, err
		}
		if !found {
			break
		}
		if r.routeReserved != nil && !verified[routeID] {
			reserved, err := r.routeReserved(ctx, routeID)
			if err != nil {
				return 0, fmt.Errorf("zone queue: check route reservation: %w", err)
			}
			if !reserved {
				unavailableRoutes = append(unavailableRoutes, routeID)
				continue
			}
			verified[routeID] = true
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
		  WHERE deleted_at IS NULL AND zone_id = $1::uuid AND delivery_date = $2::date
		    AND status = 'PENDING' AND route_id IS NULL
		    AND (queued_at, id) <= (
		         SELECT queued_at, id FROM orders
		          WHERE id = $3::uuid AND deleted_at IS NULL AND status = 'PENDING'
		            AND route_id IS NULL AND zone_id = $1::uuid AND delivery_date = $2::date
		        )`,
		order.ZoneID, order.DeliveryDate.String(), order.ID,
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
func routeWithRoom(ctx context.Context, tx pgx.Tx, zoneID, deliveryDate string, limits domain.ZoneLimits, unavailableRoutes []string) (string, int, bool, error) {
	zoneActive, err := activeInZone(ctx, tx, zoneID, deliveryDate)
	if err != nil {
		return "", 0, false, fmt.Errorf("zone queue: zone active: %w", err)
	}
	if zoneActive >= limits.Daily {
		return "", 0, false, nil
	}

	var routeID string
	var lastSequence, active int
	err = tx.QueryRow(ctx,
		`SELECT route_id,
		        COALESCE(max(stop_sequence), 0),
		        count(*) FILTER (WHERE status IN ('ROUTE_ASSIGNED', 'PICKED_UP', 'IN_TRANSIT')) AS active
		   FROM orders
		  WHERE deleted_at IS NULL AND zone_id = $1::uuid AND delivery_date = $2::date AND route_id IS NOT NULL
		    AND NOT (route_id = ANY(COALESCE($4::text[], ARRAY[]::text[])))
		  GROUP BY route_id
		  HAVING count(*) FILTER (WHERE status IN ('ROUTE_ASSIGNED', 'PICKED_UP', 'IN_TRANSIT')) < $3
		  ORDER BY active ASC, route_id ASC
		  LIMIT 1`,
		zoneID, deliveryDate, limits.PerRoute, unavailableRoutes,
	).Scan(&routeID, &lastSequence, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, fmt.Errorf("zone queue: route with room: %w", err)
	}
	return routeID, lastSequence + 1, true, nil
}

// oldestWaiting locks the oldest PENDING order of the zone and day that has no route. Waiting for row locks preserves FIFO during concurrent edits or cancellations.
func oldestWaiting(ctx context.Context, tx pgx.Tx, zoneID, deliveryDate string) (domain.Order, bool, error) {
	order, err := scanOrder(tx.QueryRow(ctx,
		`SELECT `+orderColumns+` FROM orders
		  WHERE deleted_at IS NULL AND zone_id = $1::uuid AND delivery_date = $2::date
		    AND status = 'PENDING' AND route_id IS NULL
		  ORDER BY queued_at ASC, id ASC
		  LIMIT 1
		  FOR UPDATE`,
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

func (r *OrderRepository) WaitingZoneDays(ctx context.Context) ([]domain.ZoneDay, error) {
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT zone_id::text, to_char(delivery_date, 'YYYY-MM-DD') AS day FROM orders
		WHERE deleted_at IS NULL AND status = 'PENDING' AND route_id IS NULL AND zone_id IS NOT NULL
		ORDER BY day, zone_id::text`)
	if err != nil {
		return nil, fmt.Errorf("zone queue: waiting zone days: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[domain.ZoneDay])
}

func (r *OrderRepository) ZoneActiveCount(ctx context.Context, zoneID, deliveryDate string) (int, error) {
	return activeInZone(ctx, r.pool, zoneID, deliveryDate)
}

const activeOrderStatuses = "('ROUTE_ASSIGNED', 'PICKED_UP', 'IN_TRANSIT')"

func activeInZone(ctx context.Context, db interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, zoneID, deliveryDate string) (int, error) {
	var active int
	err := db.QueryRow(ctx, `SELECT count(*) FROM orders
		WHERE deleted_at IS NULL AND zone_id = $1::uuid AND delivery_date = $2::date
		AND status IN `+activeOrderStatuses, zoneID, deliveryDate).Scan(&active)
	return active, err
}
