package domain

import "testing"

func TestCustomerStatus_Valid(t *testing.T) {
	cases := []CustomerStatus{StatusActive, StatusBlockedSurcharge, StatusBlockedIncidents}
	for _, s := range cases {
		if !s.Valid() {
			t.Fatalf("status %q debe ser válido", string(s))
		}
	}
}

func TestCustomerStatus_Invalid(t *testing.T) {
	if CustomerStatus("UNKNOWN").Valid() {
		t.Fatal("status desconocido debe ser inválido")
	}
}

func TestCustomerStatus_CanCreateOrders(t *testing.T) {
	if !StatusActive.CanCreateOrders() {
		t.Fatal("ACTIVO debe poder crear pedidos")
	}
	if StatusBlockedSurcharge.CanCreateOrders() {
		t.Fatal("BLOQUEADO_POR_SALDO NO debe poder crear pedidos")
	}
	if StatusBlockedIncidents.CanCreateOrders() {
		t.Fatal("BLOQUEADO_POR_INCIDENCIAS NO debe poder crear pedidos")
	}
}
