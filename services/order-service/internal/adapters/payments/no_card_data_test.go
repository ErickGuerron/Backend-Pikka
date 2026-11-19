package payments_test

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

var cardDataName = regexp.MustCompile(`(?i)^(card.*|.*card_?(number|holder|brand|expir.*)|cvv2?|cvc2?|pan|last_?4|last_?four|expir(y|ation).*|security_?code)$`)

// ADR 0006 seccion 3: lo que entra y sale de la pasarela no tiene donde llevar numero, vencimiento ni CVV.
// Method (CASH o CARD) es la preferencia del cliente, no un dato de tarjeta.
func TestGatewayTypesHaveNoRoomForCardData(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(ports.ChargeRequest{}),
		reflect.TypeOf(ports.RefundRequest{}),
		reflect.TypeOf(domain.PaymentAttempt{}),
	} {
		for i := 0; i < typ.NumField(); i++ {
			if name := typ.Field(i).Name; cardDataName.MatchString(name) {
				t.Errorf("%s.%s looks like card data", typ.Name(), name)
			}
		}
	}
}

func TestGatewayCardDataDetectorIsSensitive(t *testing.T) {
	for _, bad := range []string{"CardNumber", "CVV", "Pan", "ExpiryDate", "Last4", "CardHolder"} {
		if !cardDataName.MatchString(bad) {
			t.Errorf("%q must be flagged", bad)
		}
	}
	for _, ok := range []string{"Amount", "Method", "Status", "Reference", "CustomerID", "CreatedAt"} {
		if cardDataName.MatchString(ok) {
			t.Errorf("%q must not be flagged", ok)
		}
	}
}

// Every status the system can produce says it is simulated (ADR 0006, section 4).
func TestEveryPaymentStatusIsSimulated(t *testing.T) {
	for _, status := range []domain.PaymentStatus{domain.PaymentApproved, domain.PaymentDeclined} {
		if got := string(status); len(got) < 10 || got[:10] != "SIMULATED_" {
			t.Errorf("status %q must start with SIMULATED_", got)
		}
	}
}
