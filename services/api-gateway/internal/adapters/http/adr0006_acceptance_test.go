package http_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/infrastructure/jwt"
)

// Claves YAML (propiedades o parametros) que serian datos de tarjeta.
var cardDataKey = regexp.MustCompile(`(?im)^\s*"?(card\w*|\w*card_?(number|holder|brand|expir\w*)|cvv2?|cvc2?|pan|last_?4|last_?four|expir(y|ation)\w*|security_?code)"?\s*:`)

func servedOpenAPI(t *testing.T) string {
	t.Helper()
	opts := httpapi.Options{
		CORSAllowedOrigins: []string{"http://localhost:5173"},
		RateLimitPerSecond: 1000, RateLimitBurst: 1000, LoginRatePerMinute: 6000,
		RequestTimeout: time.Second, BodyLimit: "1K", EnableDocs: true,
	}
	h := httpapi.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), opts, jwt.NewVerifier(secret, "lastmile-auth"), &fakeAuth{}, nil, func(context.Context) bool { return true })
	rec := do(h, http.MethodGet, "/openapi.yaml", "", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	return rec.Body.String()
}

// ADR 0006 "Consecuencias": ninguna respuesta lleva datos de tarjeta. Se revisa el contrato que el
// Gateway publica, que es lo que el frontend consume.
func TestADR0006_ThePublishedContractHasNoCardDataFields(t *testing.T) {
	spec := servedOpenAPI(t)

	assert.Greater(t, strings.Count(spec, "operationId:"), 30, "se reviso el contrato completo")
	assert.Empty(t, cardDataKey.FindAllString(spec, -1), "el contrato publicado no debe tener campos de tarjeta")
}

// El detector de arriba encuentra lo que debe.
func TestADR0006_TheCardDataDetectorIsSensitive(t *testing.T) {
	for _, bad := range []string{"  cardNumber: { type: string }", "    cvv: { type: string }", "  expiryDate: x", "  card_holder: x", "  last4: x", "  pan: x"} {
		assert.NotEmpty(t, cardDataKey.FindString(bad), bad)
	}
	for _, ok := range []string{"  amount: { type: string }", "  reference: { type: string }", "  status: { type: string }", "  panel: x", "  expiresAt: x", "  simulated: { type: boolean }"} {
		assert.Empty(t, cardDataKey.FindString(ok), ok)
	}
}

// "el cobro simulado nunca se presenta como cargo real" (ADR 0006, seccion 4): las dos respuestas con
// dinero del cliente declaran `simulated` como obligatorio, para que ninguna app pueda ignorarlo.
func TestADR0006_MoneyResponsesAlwaysDeclareThemSimulated(t *testing.T) {
	spec := servedOpenAPI(t)

	for _, schema := range []string{"BalancePayment", "MyBalance"} {
		start := strings.Index(spec, "\n    "+schema+":\n")
		require.GreaterOrEqual(t, start, 0, schema)
		block := spec[start+1:]
		// El bloque del esquema termina en la siguiente clave de su mismo nivel.
		head := block
		if cut := regexp.MustCompile(`\n    \w+:\n`).FindStringIndex(block[1:]); cut != nil {
			head = block[:cut[0]+1]
		}
		assert.Regexp(t, `required: \[[^\]]*\bsimulated\b[^\]]*\]`, head, "%s debe exigir simulated", schema)
	}
}
