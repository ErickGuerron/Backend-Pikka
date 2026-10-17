package config

import (
	"testing"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"ROUTING_GRPC_PORT", "ORDER_SERVICE_ADDR", "ZONE_SERVICE_ADDR", "DRIVER_SERVICE_ADDR"} {
		t.Setenv(key, "")
	}
}

func TestLoadUsesDefaults(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GRPCPort != 9094 {
		t.Fatalf("GRPCPort = %d, want 9094", cfg.GRPCPort)
	}
	if cfg.OrderServiceAddr != "localhost:9091" || cfg.ZoneServiceAddr != "localhost:9090" || cfg.DriverServiceAddr != "localhost:9092" {
		t.Fatalf("addresses = %+v", cfg)
	}
}

func TestLoadReadsEnvironment(t *testing.T) {
	clearEnv(t)
	t.Setenv("ROUTING_GRPC_PORT", "9199")
	t.Setenv("ORDER_SERVICE_ADDR", "order-service:9091")
	t.Setenv("ZONE_SERVICE_ADDR", "zone-service:9090")
	t.Setenv("DRIVER_SERVICE_ADDR", "driver-service:9092")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.GRPCPort != 9199 || cfg.OrderServiceAddr != "order-service:9091" ||
		cfg.ZoneServiceAddr != "zone-service:9090" || cfg.DriverServiceAddr != "driver-service:9092" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	for _, raw := range []string{"abc", "0", "65536", "-1"} {
		t.Run(raw, func(t *testing.T) {
			clearEnv(t)
			t.Setenv("ROUTING_GRPC_PORT", raw)

			if _, err := Load(); err == nil {
				t.Fatalf("Load accepted ROUTING_GRPC_PORT=%q", raw)
			}
		})
	}
}
