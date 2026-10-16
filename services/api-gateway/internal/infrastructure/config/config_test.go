package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setValidSecret deja un JWT_SECRET válido para que Load no falle por ese único campo.
func setValidSecret(t *testing.T) {
	t.Helper()
	t.Setenv("JWT_SECRET", strings.Repeat("s", minJWTSecretLen))
}

func TestLoad_TrackingServiceAddr(t *testing.T) {
	cases := []struct {
		name string
		env  string
		set  bool
		want string
	}{
		{name: "defaults to the local tracking port when unset", set: false, want: "localhost:9093"},
		{name: "reads TRACKING_GRPC_ADDR when set", env: "tracking-service:9093", set: true, want: "tracking-service:9093"},
		{name: "empty value falls back to the default", env: "", set: true, want: "localhost:9093"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setValidSecret(t)
			if tc.set {
				t.Setenv("TRACKING_GRPC_ADDR", tc.env)
			}

			cfg, err := Load()

			require.NoError(t, err)
			assert.Equal(t, tc.want, cfg.TrackingServiceAddr)
		})
	}
}
