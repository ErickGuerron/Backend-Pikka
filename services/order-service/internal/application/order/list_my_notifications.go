package order

import (
	"context"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// ListMyNotifications devuelve los avisos del cliente autenticado (ADR 0007, section 3).
type ListMyNotifications struct {
	customers     ports.CustomerRepository
	notifications ports.NotificationRepository
}

// NewListMyNotifications inyecta los repositorios.
func NewListMyNotifications(customers ports.CustomerRepository, notifications ports.NotificationRepository) *ListMyNotifications {
	return &ListMyNotifications{customers: customers, notifications: notifications}
}

// Run devuelve los avisos del cliente de la sesión, nunca los de otro cliente.
func (uc *ListMyNotifications) Run(ctx context.Context, cmd Command) ([]domain.Notification, error) {
	if cmd.Role != RoleCustomer {
		return nil, domain.ErrNotCustomer
	}
	customer, err := uc.customers.GetByUserID(ctx, cmd.UserID)
	if err != nil {
		return nil, err
	}
	return uc.notifications.ListByCustomer(ctx, customer.ID)
}
