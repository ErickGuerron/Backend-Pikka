package persistence

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

func (r *OrderRepository) EditForCustomer(ctx context.Context, orderID string, customerID uuid.UUID, decide func(domain.Order) (domain.Order, error)) (domain.Order, error) {
	id, err := uuid.Parse(orderID)
	if err != nil {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Order{}, fmt.Errorf("edit order: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	current, err := scanOrder(tx.QueryRow(ctx, `SELECT `+orderColumns+` FROM orders
		WHERE id = $1 AND customer_id = $2 AND deleted_at IS NULL FOR UPDATE`, id, customerID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	if err != nil {
		return domain.Order{}, fmt.Errorf("edit order: load: %w", err)
	}
	next, err := decide(current)
	if err != nil {
		return domain.Order{}, err
	}
	zoneID, err := optionalUUID(next.ZoneID)
	if err != nil {
		return domain.Order{}, err
	}
	out, err := scanOrder(tx.QueryRow(ctx, `UPDATE orders SET
		address = $2, latitude = $3, longitude = $4, zone_id = $5, delivery_date = $6::date,
		window_start = $7::time, window_end = $8::time, address_reference = $9, postal_code = $10,
		contact_phone = $11, version = version + 1, updated_at = now(),
		queued_at = CASE WHEN zone_id IS DISTINCT FROM $5 OR delivery_date <> $6::date THEN clock_timestamp() ELSE queued_at END
		WHERE id = $1 RETURNING `+orderColumns,
		id, next.Address, next.Latitude, next.Longitude, zoneID, next.DeliveryDate.String(),
		next.Window.Start(), next.Window.End(), next.AddressReference, next.PostalCode, next.ContactPhone))
	if err != nil {
		return domain.Order{}, fmt.Errorf("edit order: update: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Order{}, fmt.Errorf("edit order: commit: %w", err)
	}
	return out, nil
}
