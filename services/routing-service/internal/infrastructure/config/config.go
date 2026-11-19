// Package config reads the Routing Service configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config groups the process configuration.
type Config struct {
	// GRPCPort serves routing.v1 to the Gateway (internal traffic).
	GRPCPort int
	// OrderServiceAddr, ZoneServiceAddr and DriverServiceAddr are the gRPC endpoints of the
	// services the planner depends on.
	OrderServiceAddr  string
	ZoneServiceAddr   string
	DriverServiceAddr string
	// TrackingServiceAddr serves the drivers' last positions, used to send each route to the driver nearest its zone.
	TrackingServiceAddr string
}

// Load builds the configuration from the environment. Missing variables fall back to the
// local defaults, the same convention the Gateway uses for its service addresses.
func Load() (Config, error) {
	port, err := intEnv("ROUTING_GRPC_PORT", 9094)
	if err != nil {
		return Config{}, err
	}
	if port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("ROUTING_GRPC_PORT must be between 1 and 65535, got %d", port)
	}
	return Config{
		GRPCPort:            port,
		OrderServiceAddr:    getEnv("ORDER_SERVICE_ADDR", "localhost:9091"),
		ZoneServiceAddr:     getEnv("ZONE_SERVICE_ADDR", "localhost:9090"),
		DriverServiceAddr:   getEnv("DRIVER_SERVICE_ADDR", "localhost:9092"),
		TrackingServiceAddr: getEnv("TRACKING_SERVICE_ADDR", "localhost:9093"),
	}, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func intEnv(key string, fallback int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", key, err)
	}
	return v, nil
}
