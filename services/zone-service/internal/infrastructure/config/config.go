// Package config lee la configuración del Zone Service desde variables de entorno.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
)

// Config agrupa la configuración del proceso.
type Config struct {
	GRPCPort int
	HTTPPort int
	Postgres PostgresConfig
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

// Load construye la configuración desde el entorno. Falla si falta la contraseña
// de la base: nunca se usa un valor por defecto para credenciales.
func Load() (Config, error) {
	grpcPort, err := intEnv("GRPC_PORT", 9090)
	if err != nil {
		return Config{}, err
	}
	httpPort, err := intEnv("HTTP_PORT", 8080)
	if err != nil {
		return Config{}, err
	}

	password := os.Getenv("POSTGRES_PASSWORD")
	if password == "" {
		return Config{}, fmt.Errorf("POSTGRES_PASSWORD is required")
	}

	return Config{
		GRPCPort: grpcPort,
		HTTPPort: httpPort,
		Postgres: PostgresConfig{
			Host:     getEnv("POSTGRES_HOST", "localhost"),
			Port:     getEnv("POSTGRES_PORT", "5432"),
			DB:       getEnv("POSTGRES_DB", "last_mile"),
			User:     getEnv("POSTGRES_USER", "zone_service_user"),
			Password: password,
			Schema:   getEnv("POSTGRES_SCHEMA", "zones"),
		},
	}, nil
}

// DSN devuelve la URL de conexión. search_path incluye public porque PostGIS
// vive en ese esquema: sin él, el tipo geography no se resuelve.
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

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func intEnv(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return n, nil
}
