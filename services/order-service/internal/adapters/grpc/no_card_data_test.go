package grpc

import (
	"regexp"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	customerv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/customer/v1"
	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
)

// ADR 0006 seccion 3 y "Consecuencias": ninguna respuesta (ni peticion) del contrato lleva datos de tarjeta.
// Se revisan todos los campos de todos los mensajes de orders.v1 y customer.v1.
var cardDataField = regexp.MustCompile(`(?i)^(card.*|.*card_?(number|holder|brand|expir.*)|cvv2?|cvc2?|pan|last_?4|last_?four|expir(y|ation).*|security_?code)$`)

func TestNoMessageOfTheContractCarriesCardData(t *testing.T) {
	for _, file := range []protoreflect.FileDescriptor{ordersv1.File_orders_v1_orders_proto, customerv1.File_customer_v1_customer_proto} {
		messages := file.Messages()
		checked := 0
		for i := 0; i < messages.Len(); i++ {
			msg := messages.Get(i)
			fields := msg.Fields()
			for j := 0; j < fields.Len(); j++ {
				checked++
				if name := string(fields.Get(j).Name()); cardDataField.MatchString(name) {
					t.Errorf("%s.%s looks like card data", msg.Name(), name)
				}
			}
		}
		if checked < 20 {
			t.Fatalf("%s: only %d fields checked, the scan is not looking at the whole contract", file.Path(), checked)
		}
	}
}

// The scan catches what it should.
func TestCardDataDetectorIsSensitive(t *testing.T) {
	for _, bad := range []string{"card_number", "cvv", "cvc", "pan", "card_holder", "expiry_date", "expiration_month", "last4", "last_four", "security_code", "card_brand"} {
		if !cardDataField.MatchString(bad) {
			t.Errorf("%q must be flagged", bad)
		}
	}
	for _, ok := range []string{"amount", "reference", "status", "created_at", "pending_surcharge", "unit_price", "applied_by", "unblocked", "simulated"} {
		if cardDataField.MatchString(ok) {
			t.Errorf("%q must not be flagged", ok)
		}
	}
}
