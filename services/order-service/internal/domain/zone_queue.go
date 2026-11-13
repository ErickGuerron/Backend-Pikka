package domain

// OccupiesRouteSlot reports whether an order in this status holds a slot of its zone's driver
// route. Those are the orders that count against max_orders_per_route. PENDING orders wait in
// line; DELIVERED, NOT_DELIVERED and CANCELLED release their slot.
func (s Status) OccupiesRouteSlot() bool {
	switch s {
	case StatusRouteAssigned, StatusPickedUp, StatusInTransit:
		return true
	default:
		return false
	}
}

// ZoneLimits separates the zone's daily capacity from the capacity of one route.
type ZoneLimits struct {
	Daily    int
	PerRoute int
}

type ZoneDay struct {
	ZoneID       string
	DeliveryDate string
}
