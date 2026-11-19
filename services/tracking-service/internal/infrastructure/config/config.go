// Package config carga la configuración del Tracking Service desde variables de entorno.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
)

// Config es la configuración del Tracking Service.
type Config struct {
	// GRPCPort es el puerto del servidor TrackingService. Nunca se publica al host.
	GRPCPort int
	// RedisAddr es host:puerto del Redis que guarda la última posición por repartidor.
	RedisAddr string
	// RedisPassword es obligatoria: el Redis de compose exige contraseña.
	RedisPassword string
	// OSRMURL es la base del motor de rutas, por ejemplo http://osrm:5000.
	OSRMURL string
}

// Load lee y valida la configuración. Una variable vacía cuenta como ausente.
func Load() (Config, error) {
	var errs []error
	cfg := Config{
		GRPCPort:      getInt("TRACKING_GRPC_PORT", 9093, &errs),
		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		OSRMURL:       getEnv("OSRM_URL", "http://localhost:5000"),
	}
	if cfg.GRPCPort < 1 || cfg.GRPCPort > 65535 {
		errs = append(errs, errors.New("TRACKING_GRPC_PORT must be between 1 and 65535"))
	}
	if cfg.RedisPassword == "" {
		errs = append(errs, errors.New("REDIS_PASSWORD is required"))
	}
	if !isHTTPURL(cfg.OSRMURL) {
		errs = append(errs, errors.New("OSRM_URL must be an http or https URL"))
	}
	return cfg, errors.Join(errs...)
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getInt(key string, def int, errs *[]error) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s must be an integer", key))
		return def
	}
	return n
}
