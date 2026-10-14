package domain

import "errors"

var (
	// ErrZoneNotCovered: no hay zona activa que contenga la dirección (ADR 0007, section 3).
	ErrZoneNotCovered = errors.New("address is outside every served zone")

	// ErrZonesUnavailable: Zone Service no respondió; el pedido no se registra a ciegas.
	ErrZonesUnavailable = errors.New("zone service is unavailable")

	// ErrNotCustomer: solo el rol CUSTOMER registra pedidos desde la app móvil.
	ErrNotCustomer = errors.New("only customers can register orders")
)
