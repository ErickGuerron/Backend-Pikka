package domain

// CustomerStatus modela el estado de cuenta de un cliente en el esquema orders.
//
// Los valores posibles quedan congelados en la consigna del feature
// (customer-mvp-backend, BE-T4b) y se reflejan en un CHECK constraint en la
// migración 000001_create_customers.
type CustomerStatus string

const (
	// StatusActive: el cliente puede crear pedidos y acumular recargos.
	StatusActive CustomerStatus = "ACTIVO"
	// StatusBlockedSurcharge: el cliente no puede crear pedidos porque
	// tiene recargos impagos.
	StatusBlockedSurcharge CustomerStatus = "BLOQUEADO_POR_SALDO"
	// StatusBlockedIncidents: bloqueado por motivos operativos
	// (incidencias reportadas, disputa abierta, etc.).
	StatusBlockedIncidents CustomerStatus = "BLOQUEADO_POR_INCIDENCIAS"
)

// Valid reporta si el status es uno de los valores conocidos.
func (s CustomerStatus) Valid() bool {
	switch s {
	case StatusActive, StatusBlockedSurcharge, StatusBlockedIncidents:
		return true
	default:
		return false
	}
}

// CanCreateOrders devuelve true únicamente para clientes ACTIVO.
// Es la regla de negocio del feature: cualquier bloqueo, sea por saldo o por
// incidencias, impide la creación de pedidos.
func (s CustomerStatus) CanCreateOrders() bool {
	return s == StatusActive
}
