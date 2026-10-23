package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/infrastructure/config"
)

func TestLoadUsaDefectoSinVariable(t *testing.T) {
	t.Setenv("DRIVER_DB_PASSWORD", "secreto")
	t.Setenv("DRIVER_ARRIVAL_RADIUS_METERS", "")

	cfg, err := config.Load()

	require.NoError(t, err)
	assert.Equal(t, domain.DefaultArrivalRadiusMeters, cfg.ArrivalRadiusMeters)
	assert.Equal(t, "driver_service_user", cfg.Postgres.User)
	assert.Equal(t, "drivers", cfg.Postgres.Schema)
}

func TestLoadLeeRadioEnMetros(t *testing.T) {
	t.Setenv("DRIVER_DB_PASSWORD", "secreto")
	t.Setenv("DRIVER_ARRIVAL_RADIUS_METERS", "75.5")

	cfg, err := config.Load()

	require.NoError(t, err)
	assert.Equal(t, 75.5, cfg.ArrivalRadiusMeters)
}

func TestLoadExigeContrasenaDeBase(t *testing.T) {
	t.Setenv("DRIVER_DB_PASSWORD", "")

	_, err := config.Load()

	assert.Error(t, err)
}

func TestDSNFijaEsquemaYSearchPath(t *testing.T) {
	p := config.PostgresConfig{Host: "db", Port: "5432", DB: "last_mile", User: "driver_service_user", Password: "x", Schema: "drivers"}

	dsn := p.DSN()

	assert.Contains(t, dsn, "search_path=drivers%2Cpublic")
	assert.Contains(t, dsn, "driver_service_user:x@db:5432/last_mile")
}

func TestParseRadiusRechazaValoresInvalidos(t *testing.T) {
	for _, raw := range []string{"abc", "0", "-10", "NaN", "Inf"} {
		t.Run(raw, func(t *testing.T) {
			_, err := config.ParseRadius(raw)
			assert.Error(t, err)
		})
	}
}

func TestLoadLeeLaDireccionDeOrdersYVaciaPorDefecto(t *testing.T) {
	t.Setenv("DRIVER_DB_PASSWORD", "secreto")
	t.Setenv("ORDERS_GRPC_ADDR", "")

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "", cfg.OrdersGRPCAddr)

	t.Setenv("ORDERS_GRPC_ADDR", "order-service:9091")
	cfg, err = config.Load()
	require.NoError(t, err)
	assert.Equal(t, "order-service:9091", cfg.OrdersGRPCAddr)
}
