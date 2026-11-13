package domain

import (
	"fmt"
	"strings"
)

// PlannableOrder es un pedido con su zona ya resuelta, listo para planificar.
type PlannableOrder struct {
	OrderID string
	ZoneID  string
	Version int64
	// Address is the delivery address as Order Service stored it. It is carried to the stop.
	Address string
	// WindowEnd is the local HH:mm delivery deadline.
	WindowEnd string
}

// Validate comprueba que el pedido tenga order_id y zone_id no vacios.
func (o PlannableOrder) Validate() error {
	if strings.TrimSpace(o.OrderID) == "" {
		return fmt.Errorf("%w: order id is blank", ErrInvalidOrder)
	}
	if strings.TrimSpace(o.ZoneID) == "" {
		return fmt.Errorf("%w: zone id is blank for order %q", ErrInvalidOrder, o.OrderID)
	}
	return nil
}
