package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// ListPendingByDate devuelve los pedidos PENDING de una fecha de entrega, del más antiguo al más reciente.
func (r *OrderRepository) ListPendingByDate(ctx context.Context, deliveryDate string) ([]domain.Order, error) {
	return r.queryOrders(ctx,
		`SELECT `+orderColumns+` FROM orders
		  WHERE deleted_at IS NULL AND status = 'PENDING' AND route_id IS NULL AND delivery_date = $1::date
		  ORDER BY queued_at ASC, id ASC`,
		deliveryDate,
	)
}

// AssignToRoute bloquea las filas en orden de id (evita interbloqueos entre llamadas concurrentes),
// aplica decide a cada pedido y los actualiza en la misma transacción. Cualquier error revierte todo.
func (r *OrderRepository) AssignToRoute(ctx context.Context, orderIDs []string, decide func(domain.Order) (domain.Order, error)) ([]domain.Order, error) {
	return r.assignToRoute(ctx, orderIDs, "", nil, decide)
}

func (r *OrderRepository) AssignWithinCapacity(ctx context.Context, orderIDs []string, routeID string, capacity ports.ZoneCapacity, decide func(domain.Order) (domain.Order, error)) ([]domain.Order, error) {
	return r.assignToRoute(ctx, orderIDs, routeID, capacity, decide)
}

func (r *OrderRepository) assignToRoute(ctx context.Context, orderIDs []string, routeID string, capacity ports.ZoneCapacity, decide func(domain.Order) (domain.Order, error)) ([]domain.Order, error) {
	ids := make([]uuid.UUID, 0, len(orderIDs))
	for _, raw := range orderIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, domain.ErrOrderNotFound
		}
		ids = append(ids, id)
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("assign route: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	lock := lockOrders
	if capacity != nil {
		lock = func(ctx context.Context, tx pgx.Tx, ids []uuid.UUID) (map[string]domain.Order, error) {
			return lockCapacityOrders(ctx, tx, ids, routeID, capacity)
		}
	}
	current, err := lock(ctx, tx, ids)
	if err != nil {
		return nil, err
	}

	next := make([]domain.Order, 0, len(ids))
	for _, id := range ids {
		order, ok := current[id.String()]
		if !ok {
			return nil, domain.ErrOrderNotFound
		}
		updated, err := decide(order)
		if err != nil {
			return nil, fmt.Errorf("%w: order %s", err, id)
		}
		next = append(next, updated)
	}

	out := make([]domain.Order, 0, len(next))
	for _, order := range next {
		saved, err := scanOrder(tx.QueryRow(ctx,
			`UPDATE orders
			    SET status = $2, review_reasons = $3, route_id = $4, stop_sequence = $5,
			        version = version + 1, updated_at = now()
			  WHERE id = $1
			RETURNING `+orderColumns,
			order.ID, string(order.Status), reasonStrings(order.ReviewReasons), order.RouteID,
			optionalSequence(order.StopSequence),
		))
		if isDuplicateStopSequence(err) {
			return nil, domain.ErrDuplicateStopSequence
		}
		if err != nil {
			return nil, fmt.Errorf("assign route: update: %w", err)
		}
		out = append(out, saved)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("assign route: commit: %w", err)
	}
	return out, nil
}

// optionalSequence guarda 0 (sin secuencia) como NULL, así el índice único ignora los pedidos sin parada.
func optionalSequence(sequence int) any {
	if sequence == 0 {
		return nil
	}
	return sequence
}

// isDuplicateStopSequence detecta la violación del índice único de paradas (SQLSTATE 23505 en esa restricción).
func isDuplicateStopSequence(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "orders_route_stop_sequence_key"
}

// lockOrders carga y bloquea los pedidos pedidos, indexados por id.
func lockOrders(ctx context.Context, tx pgx.Tx, ids []uuid.UUID) (map[string]domain.Order, error) {
	rows, err := tx.Query(ctx,
		`SELECT `+orderColumns+` FROM orders WHERE deleted_at IS NULL AND id = ANY($1::uuid[]) ORDER BY id FOR UPDATE`, ids)
	if err != nil {
		return nil, fmt.Errorf("assign route: lock: %w", err)
	}
	defer rows.Close()

	out := make(map[string]domain.Order, len(ids))
	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("assign route: scan: %w", err)
		}
		out[order.ID] = order
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("assign route: rows: %w", err)
	}
	return out, nil
}

func lockCapacityOrders(ctx context.Context, tx pgx.Tx, ids []uuid.UUID, routeID string, capacity ports.ZoneCapacity) (map[string]domain.Order, error) {
	if len(ids) == 0 {
		return nil, domain.ErrNoOrdersToAssign
	}
	var zoneID, date string
	err := tx.QueryRow(ctx, `SELECT COALESCE(zone_id::text, ''), to_char(delivery_date, 'YYYY-MM-DD') FROM orders
		WHERE id = $1 AND deleted_at IS NULL`, ids[0]).Scan(&zoneID, &date)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	if zoneID == "" {
		return nil, domain.ErrZoneCapacityChanged
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, zoneID+"|"+date); err != nil {
		return nil, err
	}
	current, err := lockOrders(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		o, ok := current[id.String()]
		if !ok {
			return nil, domain.ErrOrderNotFound
		}
		if o.Status != domain.StatusPending || o.RouteID != "" {
			return nil, domain.ErrOrderNotPending
		}
		if o.ZoneID != zoneID || o.DeliveryDate.String() != date {
			return nil, domain.ErrZoneCapacityChanged
		}
	}
	limits, err := capacity.ZoneLimits(ctx, zoneID)
	if err != nil {
		return nil, err
	}
	active, err := activeInZone(ctx, tx, zoneID, date)
	if err != nil {
		return nil, err
	}
	free := limits.Daily - active
	if free < len(ids) {
		return nil, domain.ErrZoneCapacityChanged
	}
	var activeOnRoute int
	var differentScope bool
	if err := tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE status IN `+activeOrderStatuses+`),
		COALESCE(bool_or(zone_id IS DISTINCT FROM $2::uuid OR delivery_date <> $3::date), false)
		FROM orders WHERE route_id = $1 AND deleted_at IS NULL`, routeID, zoneID, date).Scan(&activeOnRoute, &differentScope); err != nil {
		return nil, err
	}
	if differentScope || activeOnRoute+len(ids) > limits.PerRoute {
		return nil, domain.ErrRouteCapacityChanged
	}
	var admitted int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT id FROM orders WHERE deleted_at IS NULL AND status = 'PENDING' AND route_id IS NULL
		  AND zone_id = $1::uuid AND delivery_date = $2::date
		ORDER BY queued_at, id LIMIT $3
	) AS eligible WHERE id = ANY($4::uuid[])`, zoneID, date, len(ids), ids).Scan(&admitted); err != nil {
		return nil, err
	}
	if admitted != len(ids) {
		return nil, domain.ErrZoneCapacityChanged
	}
	return current, nil
}
