package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/infrastructure/config"
)

func TestLoadUsaDefectoSinVariable(t *testing.T) {
	t.Setenv("DRIVER_ARRIVAL_RADIUS_METERS", "")

	cfg, err := config.Load()

	require.NoError(t, err)
	assert.Equal(t, domain.DefaultArrivalRadiusMeters, cfg.ArrivalRadiusMeters)
}

func TestLoadLeeRadioEnMetros(t *testing.T) {
	t.Setenv("DRIVER_ARRIVAL_RADIUS_METERS", "75.5")

	cfg, err := config.Load()

	require.NoError(t, err)
	assert.Equal(t, 75.5, cfg.ArrivalRadiusMeters)
}

func TestParseRadiusRechazaValoresInvalidos(t *testing.T) {
	for _, raw := range []string{"abc", "0", "-10", "NaN", "Inf"} {
		t.Run(raw, func(t *testing.T) {
			_, err := config.ParseRadius(raw)
			assert.Error(t, err)
		})
	}
}
