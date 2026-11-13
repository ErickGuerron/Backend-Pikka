package order

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

type CustomerOrderEditor interface {
	EditForCustomer(context.Context, string, uuid.UUID, func(domain.Order) (domain.Order, error)) (domain.Order, error)
}

type EditMyOrder struct {
	customers ports.CustomerRepository
	orders    CustomerOrderEditor
	zones     ports.ZoneClassifier
	location  *time.Location
	queue     *ZoneQueue
}

func NewEditMyOrder(customers ports.CustomerRepository, orders CustomerOrderEditor, zones ports.ZoneClassifier, location *time.Location, queue *ZoneQueue) *EditMyOrder {
	return &EditMyOrder{customers: customers, orders: orders, zones: zones, location: location, queue: queue}
}

func (uc *EditMyOrder) Run(ctx context.Context, userID uuid.UUID, role, orderID string, edit domain.CustomerOrderEdit) (domain.Order, error) {
	if role != RoleCustomer {
		return domain.Order{}, domain.ErrNotCustomer
	}
	customer, err := uc.customers.GetByUserID(ctx, userID)
	if err != nil {
		return domain.Order{}, err
	}
	out, err := uc.orders.EditForCustomer(ctx, orderID, customer.ID, func(current domain.Order) (domain.Order, error) {
		next, err := current.EditDelivery(edit, time.Now().In(uc.location))
		if err != nil {
			return domain.Order{}, err
		}
		if edit.HasPoint() {
			next.ZoneID, err = uc.zones.ClassifyPoint(ctx, next.Latitude, next.Longitude)
			if err != nil {
				return domain.Order{}, err
			}
			if next.ZoneID == "" {
				return domain.Order{}, domain.ErrZoneNotCovered
			}
		}
		return next, nil
	})
	if err != nil {
		return domain.Order{}, err
	}
	if position, err := uc.queue.positionOf(ctx, out); err == nil {
		out.QueuePosition = position
	}
	return out, nil
}
