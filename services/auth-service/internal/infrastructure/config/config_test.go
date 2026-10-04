package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/infrastructure/config"
)

func TestLoadRequiresSecrets(t *testing.T) {
	t.Setenv("POSTGRES_PASSWORD", "")
	t.Setenv("JWT_SECRET", "short")
	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "POSTGRES_PASSWORD")
	assert.Contains(t, err.Error(), "JWT_SECRET")
}

func TestLoadValid(t *testing.T) {
	t.Setenv("POSTGRES_PASSWORD", "p@ss/word")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("JWT_TTL", "15m")
	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, 15*time.Minute, cfg.JWTTTL)
	assert.Contains(t, cfg.Postgres.DSN(), "search_path=auth")
	assert.Contains(t, cfg.Postgres.DSN(), "p%40ss%2Fword")
}

func TestLoadBootstrapPair(t *testing.T) {
	t.Setenv("POSTGRES_PASSWORD", "x")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("BOOTSTRAP_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("BOOTSTRAP_ADMIN_PASSWORD", "")
	_, err := config.Load()
	assert.ErrorContains(t, err, "BOOTSTRAP_ADMIN")
}
