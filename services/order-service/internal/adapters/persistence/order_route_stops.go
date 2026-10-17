package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

var _ ports.RouteStopRepository = (*OrderRepository)(nil)

// CountStopsBefore cuenta en una sola consulta las paradas abiertas de la misma ruta con secuencia menor.
// La fila del pedido propio filtra por cliente en el FROM: un pedido ajeno no tiene fila y responde
// domain.ErrOrderNotFound. Si el pedido no tiene ruta o secuencia, la comparación es NULL y el conteo es 0.
func (r *OrderRepository) CountStopsBefore(ctx context.Context, orderID string, customerID uuid.UUID) (int, error) {
	id, err := uuid.Parse(orderID)
	if err != nil {
		return 0, domain.ErrOrderNotFound
	}
	var count int64
	err = r.pool.QueryRow(ctx,
		`SELECT (SELECT count(*) FROM orders o
		          WHERE o.route_id = me.route_id
		            AND o.stop_sequence < me.stop_sequence
		            AND o.status NOT IN ('DELIVERED', 'NOT_DELIVERED', 'CANCELLED'))
		   FROM orders me
		  WHERE me.id = $1 AND me.customer_id = $2`,
		id, customerID,
	).Scan(&count)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, domain.ErrOrderNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("count stops before: %w", err)
	}
	return int(count), nil
}
