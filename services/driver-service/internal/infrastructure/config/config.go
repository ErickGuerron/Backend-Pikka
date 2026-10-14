// Package config lee la configuración del Driver Service desde variables de entorno.
package config

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"strconv"

	"github.com/ErickGuerron/Backend-Pikka/services/driver-service/internal/domain"
)

// Config agrupa la configuración del proceso.
type Config struct {
	HTTPPort int
	// GRPCPort expone el contrato drivers.v1 al Routing Service (tráfico interno).
	GRPCPort int
	// ArrivalRadiusMeters es la distancia máxima, en metros, entre el GPS del repartidor y
	// la dirección de la parada para que la llegada cuente como válida.
	ArrivalRadiusMeters float64
	Postgres            PostgresConfig
}

// PostgresConfig describe la conexión del usuario del servicio a su esquema.
type PostgresConfig struct {
	Host     string
	Port     string
	DB       string
	User     string
	Password string
	Schema   string
}

// Load construye la configuración desde el entorno. Falla si falta la contraseña de la
// base: nunca se usa un valor por defecto para credenciales.
func Load() (Config, error) {
	httpPort, err := intEnv("DRIVER_HTTP_PORT", 8080)
	if err != nil {
		return Config{}, err
	}
	grpcPort, err := intEnv("DRIVER_GRPC_PORT", 9092)
	if err != nil {
		return Config{}, err
	}
	radius, err := radiusEnv()
	if err != nil {
		return Config{}, err
	}
	password := os.Getenv("DRIVER_DB_PASSWORD")
	if password == "" {
		return Config{}, fmt.Errorf("DRIVER_DB_PASSWORD is required")
	}
	return Config{
		HTTPPort:            httpPort,
		GRPCPort:            grpcPort,
		ArrivalRadiusMeters: radius,
		Postgres: PostgresConfig{
			Host:     getEnv("POSTGRES_HOST", "localhost"),
			Port:     getEnv("POSTGRES_PORT", "5432"),
			DB:       getEnv("POSTGRES_DB", "last_mile"),
			User:     getEnv("DRIVER_DB_USER", "driver_service_user"),
			Password: password,
			Schema:   getEnv("DRIVER_DB_SCHEMA", "drivers"),
		},
	}, nil
}

// DSN devuelve la URL de conexión. search_path fija el esquema del servicio.
func (p PostgresConfig) DSN() string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(p.User, p.Password),
		Host:   p.Host + ":" + p.Port,
		Path:   "/" + p.DB,
	}
	q := url.Values{}
	q.Set("sslmode", "disable")
	q.Set("search_path", p.Schema+",public")
	u.RawQuery = q.Encode()
	return u.String()
}

func radiusEnv() (float64, error) {
	raw := os.Getenv("DRIVER_ARRIVAL_RADIUS_METERS")
	if raw == "" {
		return domain.DefaultArrivalRadiusMeters, nil
	}
	return ParseRadius(raw)
}

// ParseRadius convierte el texto de un radio en metros y valida que sea positivo y finito.
func ParseRadius(raw string) (float64, error) {
	radius, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("DRIVER_ARRIVAL_RADIUS_METERS must be a number: %w", err)
	}
	if math.IsNaN(radius) || math.IsInf(radius, 0) || radius <= 0 {
		return 0, fmt.Errorf("DRIVER_ARRIVAL_RADIUS_METERS must be a positive number, got %q", raw)
	}
	return radius, nil
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
