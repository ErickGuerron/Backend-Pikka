package domain

// PendingOrder is a PENDING order that has not been placed on a route yet.
type PendingOrder struct {
	OrderID string
	ZoneID  string
	Address string
	// DeliveryDate is YYYY-MM-DD.
	DeliveryDate string
}

// RouteOrder is a non-cancelled order that is already on a route.
type RouteOrder struct {
	OrderID  string
	ZoneID   string
	Sequence int
	// Open is true while the stop still has to be completed (assigned, picked up or in transit).
	Open bool
}
