package order

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// F1: el motivo viaja con el cierre de la parada, para que el repositorio lo guarde en la misma transaccion
// y un barrido pueda recuperar una incidencia que no se registro.
func TestClosingAStopCarriesThePenaltyReasonToThePersistedWrite(t *testing.T) {
	for _, reason := range []string{"CLIENT_REFUSED", "CLIENT_ABSENT", "OUT_OF_WINDOW", "WRONG_ADDRESS"} {
		id := uuid.NewString()
		repo := &outcomeRepo{current: domain.Order{ID: id, CustomerID: "cust-1", Status: domain.StatusInTransit, RouteID: "z#1"}}

		got, err := NewRecordStopOutcome(repo).Run(context.Background(), RecordStopOutcomeCommand{
			Role: RoleDriver, RouteID: "z#1", OrderID: id, Outcome: domain.StatusNotDelivered, Reason: reason,
		})

		if err != nil {
			t.Fatalf("%s: %v", reason, err)
		}
		if got.PenaltyReason != domain.NotDeliveredReason(reason) {
			t.Fatalf("%s: penalty reason = %q", reason, got.PenaltyReason)
		}
	}
}

func TestClosingAStopWithNothingToPenalizeCarriesNoReason(t *testing.T) {
	cases := []struct {
		name    string
		outcome domain.Status
		reason  string
	}{
		{"delivered", domain.StatusDelivered, ""},
		{"delivered with a stray reason", domain.StatusDelivered, "CLIENT_REFUSED"},
		{"business reason", domain.StatusNotDelivered, "PRODUCT_DAMAGED"},
		{"duplicate order", domain.StatusNotDelivered, "DUPLICATE_ORDER"},
		{"missing reason from an old caller", domain.StatusNotDelivered, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := uuid.NewString()
			repo := &outcomeRepo{current: domain.Order{ID: id, CustomerID: "cust-1", Status: domain.StatusInTransit, RouteID: "z#1"}}

			got, err := NewRecordStopOutcome(repo).Run(context.Background(), RecordStopOutcomeCommand{
				Role: RoleDriver, RouteID: "z#1", OrderID: id, Outcome: tc.outcome, Reason: tc.reason,
			})

			if err != nil || got.PenaltyReason != "" {
				t.Fatalf("penalty reason = %q, err = %v", got.PenaltyReason, err)
			}
		})
	}
}
