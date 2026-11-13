package persistence

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

func (r *OrderRepository) ReassignWrongAddress(ctx context.Context, orderID string, actorID uuid.UUID, reason string, decide func(domain.Order) (domain.Order, *domain.Notification, error)) (domain.Order, error) {
	return r.review(ctx, orderID, func(current domain.Order) (domain.Order, *domain.Notification, error) {
		if current.Status != domain.StatusNotDelivered || current.RouteID == "" || current.StopSequence < 1 {
			return domain.Order{}, nil, domain.ErrInvalidTransition
		}
		return decide(current)
	}, func(tx pgx.Tx, current domain.Order) error {
		var surchargeID uuid.UUID
		err := tx.QueryRow(ctx, `SELECT id FROM surcharges
			WHERE order_id = $1 AND reason = 'WRONG_ADDRESS' AND status = 'PENDING_VERIFICATION' FOR UPDATE`, current.ID).Scan(&surchargeID)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrSurchargeNotPendingVerification
		}
		if err != nil {
			return err
		}
		var charged bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM reassignment_charges WHERE order_id = $1)`, current.ID).Scan(&charged); err != nil {
			return err
		}
		if charged {
			return domain.ErrReassignmentChargeExists
		}
		fee, err := currentReassignmentFee(ctx, tx)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO wrong_address_reassignments
			(order_id, previous_route_id, previous_stop_sequence, previous_address, previous_delivery_date, travel_fee, actor_id, reason)
			VALUES ($1, $2, $3, $4, $5::date, $6, $7, $8)`,
			current.ID, current.RouteID, current.StopSequence, current.Address, current.DeliveryDate.String(), fee.Amount, actorID, reason)
		if isUniqueViolation(err) {
			return domain.ErrReassignmentChargeExists
		}
		return err
	})
}
