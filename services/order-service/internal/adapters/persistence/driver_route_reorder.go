package persistence

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ReorderDriverRoute moves only the open stops, reusing their original sequence slots.
// A false result means the snapshot changed while the caller was planning.
func (r *OrderRepository) ReorderDriverRoute(ctx context.Context, routeID, actorID, reason string, ordered []uuid.UUID, versions map[string]int64) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var zoneDay string
	err = tx.QueryRow(ctx, `SELECT zone_id::text || '|' || delivery_date::text
		FROM orders WHERE route_id = $1 AND deleted_at IS NULL AND zone_id IS NOT NULL LIMIT 1`, routeID).Scan(&zoneDay)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// The zone queue also takes this lock before appending an order to the route.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, zoneDay); err != nil {
		return false, err
	}

	rows, err := tx.Query(ctx, `
		SELECT id, stop_sequence, version FROM orders
		WHERE route_id = $1 AND deleted_at IS NULL AND stop_sequence IS NOT NULL
		  AND status IN ('ROUTE_ASSIGNED', 'PICKED_UP', 'IN_TRANSIT')
		ORDER BY id FOR UPDATE`, routeID)
	if err != nil {
		return false, err
	}
	type stop struct {
		id       uuid.UUID
		sequence int
		version  int64
	}
	current := make([]stop, 0, len(ordered))
	for rows.Next() {
		var s stop
		if err := rows.Scan(&s.id, &s.sequence, &s.version); err != nil {
			rows.Close()
			return false, err
		}
		current = append(current, s)
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	if len(current) != len(ordered) {
		return false, nil
	}
	sort.Slice(current, func(i, j int) bool { return current[i].sequence < current[j].sequence })
	previous := make([]uuid.UUID, len(current))
	for i, s := range current {
		version, ok := versions[s.id.String()]
		if !ok || s.version != version {
			return false, nil
		}
		previous[i] = s.id
	}
	if slices.Equal(previous, ordered) {
		return true, nil
	}
	// Release the unique sequence slots inside the transaction before swapping them.
	if _, err := tx.Exec(ctx, `UPDATE orders SET stop_sequence = NULL WHERE id = ANY($1::uuid[])`, ordered); err != nil {
		return false, err
	}
	for i, id := range ordered {
		if _, err := tx.Exec(ctx, `UPDATE orders SET stop_sequence = $2, version = version + 1, updated_at = now() WHERE id = $1`,
			id, current[i].sequence); err != nil {
			return false, fmt.Errorf("reorder route stop: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO driver_route_reorders (route_id, actor_id, reason, previous_order, new_order)
		VALUES ($1, $2, $3, $4, $5)`, routeID, actorID, reason, previous, ordered); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
