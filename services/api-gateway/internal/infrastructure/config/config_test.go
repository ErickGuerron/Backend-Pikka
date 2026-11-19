package config

import (
	"strings"
	"testing"
	"time"

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

func TestLoad_RoutingServiceAddr(t *testing.T) {
	cases := []struct {
		name string
		env  string
		set  bool
		want string
	}{
		{name: "defaults to the local routing port when unset", set: false, want: "localhost:9094"},
		{name: "reads ROUTING_SERVICE_ADDR when set", env: "routing-service:9094", set: true, want: "routing-service:9094"},
		{name: "empty value falls back to the default", env: "", set: true, want: "localhost:9094"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setValidSecret(t)
			if tc.set {
				t.Setenv("ROUTING_SERVICE_ADDR", tc.env)
			}

			cfg, err := Load()

			require.NoError(t, err)
			assert.Equal(t, tc.want, cfg.RoutingServiceAddr)
		})
	}
}

func TestLoad_TrackingPollInterval(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		set     bool
		want    time.Duration
		wantErr bool
	}{
		{name: "defaults to 5 seconds when unset", set: false, want: 5 * time.Second},
		{name: "reads TRACKING_POLL_INTERVAL when set", env: "2s", set: true, want: 2 * time.Second},
		{name: "rejects a value that is not a duration", env: "fast", set: true, wantErr: true},
		{name: "rejects zero, which would spin the poll loop", env: "0s", set: true, wantErr: true},
		{name: "rejects a negative interval", env: "-1s", set: true, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setValidSecret(t)
			if tc.set {
				t.Setenv("TRACKING_POLL_INTERVAL", tc.env)
			}

			cfg, err := Load()

			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, cfg.TrackingPollInterval)
		})
	}
}
