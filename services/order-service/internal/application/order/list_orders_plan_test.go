package order

import (
	"context"
	"testing"
)

// Plan: listar pedidos sin registros devuelve vacío, no error.
func TestPlan_ListOrders_NoRecords_EmptyNoError(t *testing.T) {
	repo := &listAllRepo{}

	got, err := NewListOrders(repo).Run(context.Background(), ListOrdersQuery{Role: RoleAdmin})

	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d orders, want 0", len(got))
	}
}
