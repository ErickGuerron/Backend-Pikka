package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/domain"
)

// Estos ejemplos son los mismos que prueba el frontend (features/zones/code.test.ts).
// Si cambia la regla, cambian los dos lados a la vez.
func TestCodePrefixFromName(t *testing.T) {
	tests := map[string]string{
		"Zona Norte":           "NORTE",
		"Norte":                "NORTE",
		"Zona":                 "ZONA",
		"  zona  sector  ":     "ZONA",
		"Centro Histórico":     "CENTRO",
		"Ñuñoa":                "NUNOA",
		"Carcelén":             "CARCEL",
		"Valle de los Chillos": "VALLE",
		"Sur 2":                "SUR",
		"123":                  "ZONA",
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, want, domain.CodePrefixFromName(name))
		})
	}
}

func TestFormatCode(t *testing.T) {
	assert.Equal(t, "NORTE-001", domain.FormatCode("NORTE", 1))
	assert.Equal(t, "NORTE-123", domain.FormatCode("NORTE", 123))
	assert.Equal(t, "NORTE-1000", domain.FormatCode("NORTE", 1000))
}

func TestValidateReason(t *testing.T) {
	_, err := domain.ValidateReason("   no   ")
	assert.ErrorIs(t, err, domain.ErrReasonRequired)

	reason, err := domain.ValidateReason("  Zona fuera de servicio por obras  ")
	assert.NoError(t, err)
	assert.Equal(t, "Zona fuera de servicio por obras", reason)
}
