package domain

import "errors"

var (
	ErrInvalidAddress     = errors.New("address must contain 5 to 300 Unicode characters without NUL")
	ErrInvalidCoordinate  = errors.New("coordinates must be finite and within WGS 84 bounds")
	ErrInvalidDate        = errors.New("delivery date must be a valid YYYY-MM-DD date")
	ErrPastDeliveryDate   = errors.New("delivery date cannot precede the local registration date")
	ErrInvalidWindow      = errors.New("window must use HH:mm with end after start on the same day")
	ErrInvalidCreatedAt   = errors.New("registration instant is required")
	ErrInvalidOrderStatus = errors.New("unknown order status")
	ErrInvalidTransition  = errors.New("order status transition is not allowed")
)

// Sentinel errors del dominio Customer. Los adaptadores los traducen a
// códigos de transporte (gRPC status codes, HTTP status) según corresponda.
var (
	// ErrCustomerNotFound: el cliente no existe en el esquema orders.
	ErrCustomerNotFound = errors.New("customer not found")

	// ErrCustomerAlreadyExists: ya existe un customer con ese user_id.
	// En uso normal el dominio garantiza unicidad vía UNIQUE INDEX.
	ErrCustomerAlreadyExists = errors.New("customer already exists")

	// ErrInvalidStatus: se intentó asignar un CustomerStatus desconocido.
	ErrInvalidStatus = errors.New("invalid customer status")

	// ErrInvalidPagination: limit u offset negativos en un listado.
	ErrInvalidPagination = errors.New("invalid pagination")

	// ErrInvalidSurcharge: el monto pasado a AddSurcharge/PaySurcharge es
	// negativo o produciría un saldo inconsistente.
	ErrInvalidSurcharge = errors.New("invalid surcharge amount")

	// ErrPendingSurcharge: el cliente no puede desbloquearse porque aún
	// tiene saldo pendiente.
	ErrPendingSurcharge = errors.New("customer has pending surcharge")
)
