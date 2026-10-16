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

	ErrInvalidPhone            = errors.New("phone must have 7 to 15 digits with an optional leading +")
	ErrInvalidCity             = errors.New("city must contain 2 to 80 characters")
	ErrInvalidAddressReference = errors.New("address reference must contain 3 to 200 characters")
	ErrInvalidPostalCode       = errors.New("postal code must contain 4 to 10 letters, digits or hyphens")
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

// Errores del catálogo y del stock (ADR 0009).
var (
	ErrInvalidProductName        = errors.New("product name must have 1 to 120 characters")
	ErrInvalidProductDescription = errors.New("product description must have at most 500 characters")
	ErrInvalidPrice              = errors.New("price must be positive with at most two decimals")
	ErrInvalidStock              = errors.New("stock cannot be negative")
	ErrStockNotTracked           = errors.New("product does not track stock")
	ErrInsufficientStock         = errors.New("not enough stock for this order")
	ErrInvalidMovement           = errors.New("stock movement does not match its type")
	ErrAdjustmentReasonRequired  = errors.New("a manual stock adjustment needs a reason")
	ErrProductInactive           = errors.New("product is not available")
	ErrInvalidQuantity           = errors.New("quantity must be between 1 and 50")
	ErrNoItems                   = errors.New("order must contain at least one product")
	ErrProductNotFound           = errors.New("product not found")
	ErrDuplicateLine             = errors.New("the same product cannot appear twice in an order")
)
