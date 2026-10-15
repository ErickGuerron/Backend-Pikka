package domain

import "errors"

// ErrOrderNotCancellable: el cliente solo cancela en PENDING o ROUTE_ASSIGNED (ADR 0007, sección 8).
var ErrOrderNotCancellable = errors.New("order can only be cancelled while it is pending or assigned to a route")

// CancelByCustomer cancela el pedido a petición del cliente. El stock se devuelve en la transacción
// de persistencia. PENDING_REVIEW no se cancela por esta vía: lo decide el operador.
func (o Order) CancelByCustomer() (Order, error) {
	if !o.Status.CanCancel() {
		return o, ErrOrderNotCancellable
	}
	o.Status = StatusCancelled
	o.ReviewReasons = nil
	return o, nil
}
