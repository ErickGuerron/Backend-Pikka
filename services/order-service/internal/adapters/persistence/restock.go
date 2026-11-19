package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// returnStock devuelve al stock las cantidades de un pedido cancelado (ADR 0009, sección 2). Se llama
// dentro de la transacción que cambia el estado a CANCELLED, así la devolución y el cambio son atómicos.
// Un producto sin control de stock no genera movimiento. Los productos se bloquean en orden de id.
func returnStock(ctx context.Context, tx pgx.Tx, orderID uuid.UUID, now time.Time) error {
	rows, err := tx.Query(ctx, `
		SELECT product_id, quantity FROM order_items WHERE order_id = $1 ORDER BY product_id`, orderID)
	if err != nil {
		return fmt.Errorf("return stock: items: %w", err)
	}
	type line struct {
		productID uuid.UUID
		quantity  int64
	}
	var lines []line
	for rows.Next() {
		var l line
		if err := rows.Scan(&l.productID, &l.quantity); err != nil {
			rows.Close()
			return fmt.Errorf("return stock: scan: %w", err)
		}
		lines = append(lines, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("return stock: rows: %w", err)
	}

	for _, l := range lines {
		var resulting *int64
		err := tx.QueryRow(ctx, `
			UPDATE products SET stock_quantity = stock_quantity + $2, updated_at = $3
			WHERE id = $1 AND stock_quantity IS NOT NULL
			RETURNING stock_quantity`, l.productID, l.quantity, now).Scan(&resulting)
		if errors.Is(err, pgx.ErrNoRows) {
			continue // sin control de stock: nada que devolver
		}
		if err != nil {
			return fmt.Errorf("return stock: update: %w", err)
		}
		oid := orderID
		movement := domain.StockMovement{
			ID:             uuid.New(),
			ProductID:      l.productID,
			Type:           domain.MovementReturn,
			Delta:          l.quantity,
			ResultingStock: *resulting,
			Reason:         "pedido cancelado",
			OrderID:        &oid,
			CreatedAt:      now,
		}
		if err := insertMovement(ctx, tx, movement); err != nil {
			return fmt.Errorf("return stock: movement: %w", err)
		}
	}
	return nil
}
