package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// ADR 0007 sección 5: solo los motivos imputables al cliente generan recargo.
func TestChargesSurchargeOnlyForClientReasons(t *testing.T) {
	for _, reason := range []string{"CLIENT_REFUSED", "CLIENT_ABSENT", "OUT_OF_WINDOW"} {
		assert.True(t, domain.NotDeliveredReason(reason).ChargesSurcharge(), reason)
	}
	for _, reason := range []string{"PRODUCT_DAMAGED", "DUPLICATE_ORDER", "", "client_refused", "OTHER"} {
		assert.False(t, domain.NotDeliveredReason(reason).ChargesSurcharge(), "%q", reason)
	}
}

// WRONG_ADDRESS no cobra solo: queda en verificación del operador (ADR 0006 enmienda, tarea T4).
func TestWrongAddressNeedsVerificationNotAutomaticCharge(t *testing.T) {
	r := domain.NotDeliveredReason("WRONG_ADDRESS")
	assert.False(t, r.ChargesSurcharge())
	assert.True(t, r.NeedsVerification())
	assert.False(t, domain.NotDeliveredReason("CLIENT_REFUSED").NeedsVerification())
}

// F1: solo los motivos que generan una incidencia se guardan para reconciliar.
func TestCreatesIncidentOnlyForClientReasonsAndWrongAddress(t *testing.T) {
	for _, reason := range []string{"CLIENT_REFUSED", "CLIENT_ABSENT", "OUT_OF_WINDOW", "WRONG_ADDRESS"} {
		assert.True(t, domain.NotDeliveredReason(reason).CreatesIncident(), reason)
	}
	for _, reason := range []string{"PRODUCT_DAMAGED", "DUPLICATE_ORDER", "", "client_refused", "OTHER"} {
		assert.False(t, domain.NotDeliveredReason(reason).CreatesIncident(), "%q", reason)
	}
}
