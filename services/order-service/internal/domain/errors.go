package domain

import "errors"

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

	// ErrInvalidSurcharge: el monto pasado a AddSurcharge/PaySurcharge es
	// negativo o produciría un saldo inconsistente.
	ErrInvalidSurcharge = errors.New("invalid surcharge amount")

	// ErrPendingSurcharge: el cliente no puede desbloquearse porque aún
	// tiene saldo pendiente.
	ErrPendingSurcharge = errors.New("customer has pending surcharge")
)
