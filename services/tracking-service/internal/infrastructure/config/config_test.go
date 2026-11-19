package config

import (
	"strings"
	"testing"
)

// clearEnv vacía las variables del servicio. Una cadena vacía cuenta como ausente, igual que en Load.
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"TRACKING_GRPC_PORT", "REDIS_ADDR", "REDIS_PASSWORD", "OSRM_URL"} {
		t.Setenv(k, "")
	}
}

func TestLoad_DefaultsWithRequiredPassword(t *testing.T) {
	clearEnv(t)
	t.Setenv("REDIS_PASSWORD", "s3cret")

	cfg, err := Load()

	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GRPCPort != 9093 {
		t.Errorf("GRPCPort = %d, want 9093", cfg.GRPCPort)
	}
	if cfg.RedisAddr != "localhost:6379" {
		t.Errorf("RedisAddr = %q, want localhost:6379", cfg.RedisAddr)
	}
	if cfg.RedisPassword != "s3cret" {
		t.Errorf("RedisPassword not read from REDIS_PASSWORD")
	}
	if cfg.OSRMURL != "http://localhost:5000" {
		t.Errorf("OSRMURL = %q, want http://localhost:5000", cfg.OSRMURL)
	}
}

func TestLoad_ReadsOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("REDIS_PASSWORD", "s3cret")
	t.Setenv("REDIS_ADDR", "redis:6379")
	t.Setenv("OSRM_URL", "https://osrm.internal:5000")
	t.Setenv("TRACKING_GRPC_PORT", "9500")

	cfg, err := Load()

	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GRPCPort != 9500 || cfg.RedisAddr != "redis:6379" || cfg.OSRMURL != "https://osrm.internal:5000" {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}

func TestLoad_Errors(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		wantSub string
	}{
		{name: "missing REDIS_PASSWORD", env: map[string]string{}, wantSub: "REDIS_PASSWORD is required"},
		{name: "non-numeric port", env: map[string]string{"REDIS_PASSWORD": "x", "TRACKING_GRPC_PORT": "abc"}, wantSub: "TRACKING_GRPC_PORT must be an integer"},
		{name: "port out of range", env: map[string]string{"REDIS_PASSWORD": "x", "TRACKING_GRPC_PORT": "70000"}, wantSub: "TRACKING_GRPC_PORT must be between 1 and 65535"},
		{name: "OSRM_URL without http scheme", env: map[string]string{"REDIS_PASSWORD": "x", "OSRM_URL": "osrm:5000"}, wantSub: "OSRM_URL must be an http or https URL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			_, err := Load()

			if err == nil {
				t.Fatalf("Load succeeded, want error containing %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q does not contain %q", err, tc.wantSub)
			}
		})
	}
}
