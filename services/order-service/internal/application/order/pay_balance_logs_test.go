package order

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"regexp"
	"testing"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

type refundFails struct{ *gatewayFake }

func (refundFails) Refund(context.Context, ports.RefundRequest) (domain.PaymentAttempt, error) {
	return domain.PaymentAttempt{}, errors.New("provider down")
}

var (
	cardWords   = regexp.MustCompile(`(?i)card|cvv|cvc|expir|pan\b|holder`)
	longNumbers = regexp.MustCompile(`\d{13,19}`)
)

// ADR 0006 "Consecuencias": ningun log contiene datos de tarjeta. Se capturan los logs de todos los caminos
// del pago, incluido el peor (el reembolso falla y alguien debe conciliar a mano), y se buscan palabras y
// numeros largos de tarjeta.
func TestPayBalanceLogsNeverContainCardData(t *testing.T) {
	var out bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	// approved
	_, _ = newPay("7.50", domain.StatusBlockedSurcharge).pay()
	// declined
	declined := newPay("7.99", domain.StatusBlockedSurcharge)
	declined.gateway.status = domain.PaymentDeclined
	_, _ = declined.pay()
	// settlement fails, refund fails: the error is logged for manual reconciliation
	worst := newPay("7.50", domain.StatusBlockedSurcharge)
	worst.settle.err = domain.ErrBalanceChanged
	worst.uc.gateway = refundFails{worst.gateway}
	_, _ = worst.pay()

	logged := out.String()
	if logged == "" {
		t.Fatal("the worst path must log the failed refund, otherwise this test checks nothing")
	}
	if m := cardWords.FindString(logged); m != "" {
		t.Errorf("log mentions %q:\n%s", m, logged)
	}
	if m := longNumbers.FindString(logged); m != "" {
		t.Errorf("log has a long number %q:\n%s", m, logged)
	}
}

// The scan catches what it should.
func TestCardDataLogScanIsSensitive(t *testing.T) {
	for _, bad := range []string{"card_number=4111111111111111", "cvv=123", "expiry=12/30", "msg=\"holder name\""} {
		if cardWords.FindString(bad) == "" && longNumbers.FindString(bad) == "" {
			t.Errorf("%q must be flagged", bad)
		}
	}
	for _, ok := range []string{"level=ERROR msg=\"payment refund not recorded; reconcile by hand\" customer=6f9619ff-8b86-d011-b42d-00c04fc964ff reference=SIM-1"} {
		if cardWords.FindString(ok) != "" || longNumbers.FindString(ok) != "" {
			t.Errorf("%q must not be flagged", ok)
		}
	}
}
