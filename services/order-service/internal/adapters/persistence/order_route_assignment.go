package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// ListPendingByDate devuelve los pedidos PENDING de una fecha de entrega, del más antiguo al más reciente.
func (r *OrderRepository) ListPendingByDate(ctx context.Context, deliveryDate string) ([]domain.Order, error) {
	return r.queryOrders(ctx,
		`SELECT `+orderColumns+` FROM orders
		  WHERE status = 'PENDING' AND delivery_date = $1::date
		  ORDER BY created_at ASC, id ASC`,
		deliveryDate,
	)
}

// AssignToRoute bloquea las filas en orden de id (evita interbloqueos entre llamadas concurrentes),
// aplica decide a cada pedido y los actualiza en la misma transacción. Cualquier error revierte todo.
func (r *OrderRepository) AssignToRoute(ctx context.Context, orderIDs []string, decide func(domain.Order) (domain.Order, error)) ([]domain.Order, error) {
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

	current, err := lockOrders(ctx, tx, ids)
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
		`SELECT `+orderColumns+` FROM orders WHERE id = ANY($1::uuid[]) ORDER BY id FOR UPDATE`, ids)
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
