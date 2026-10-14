// Package config lee la configuración del Order Service desde variables de entorno.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

// Config agrupa la configuración del proceso.
type Config struct {
	HTTPPort int
	// GRPCPort expone el dominio Customer y Order al API Gateway.
	GRPCPort int
	// ZoneServiceAddr es el destino gRPC de Zone Service (validación de zona).
	ZoneServiceAddr string
	// ZoneTimeout acota cada llamada a Zone Service.
	ZoneTimeout time.Duration
	// OperatingTimezone fija el día local de registro de pedidos (IANA).
	OperatingTimezone string
	// PhotonURL es el geocodificador (Photon). En producción, una instancia propia con datos de Ecuador.
	PhotonURL string
	// PhotonBBox acota la búsqueda: minLon,minLat,maxLon,maxLat.
	PhotonBBox string
	// GeocoderTimeout acota cada geocodificación.
	GeocoderTimeout time.Duration
	Postgres        PostgresConfig
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
	httpPort, err := intEnv("HTTP_PORT", 8080)
	if err != nil {
		return Config{}, err
	}
	grpcPort, err := intEnv("ORDER_GRPC_PORT", 9091)
	if err != nil {
		return Config{}, err
	}
	zoneTimeoutMs, err := intEnv("ZONE_GRPC_TIMEOUT_MS", 2000)
	if err != nil {
		return Config{}, err
	}
	geocoderTimeoutMs, err := intEnv("GEOCODER_TIMEOUT_MS", 5000)
	if err != nil {
		return Config{}, err
	}

	password := os.Getenv("POSTGRES_PASSWORD")
	if password == "" {
		return Config{}, fmt.Errorf("POSTGRES_PASSWORD is required")
	}

	return Config{
		HTTPPort:          httpPort,
		GRPCPort:          grpcPort,
		ZoneServiceAddr:   getEnv("ZONE_SERVICE_ADDR", "localhost:9090"),
		ZoneTimeout:       time.Duration(zoneTimeoutMs) * time.Millisecond,
		OperatingTimezone: getEnv("OPERATING_TIMEZONE", "America/Guayaquil"),
		PhotonURL:         getEnv("PHOTON_URL", "https://photon.komoot.io"),
		// Caja aproximada de Ecuador continental y Galápagos; ajustar con datos reales.
		PhotonBBox:      getEnv("PHOTON_BBOX", "-92.1,-5.1,-75.1,1.7"),
		GeocoderTimeout: time.Duration(geocoderTimeoutMs) * time.Millisecond,
		Postgres: PostgresConfig{
			Host:     getEnv("POSTGRES_HOST", "localhost"),
			Port:     getEnv("POSTGRES_PORT", "5432"),
			DB:       getEnv("POSTGRES_DB", "last_mile"),
			User:     getEnv("POSTGRES_USER", "order_service_user"),
			Password: password,
			Schema:   getEnv("POSTGRES_SCHEMA", "orders"),
		},
	}, nil
}

// DSN devuelve la URL de conexión. search_path fija el esquema del servicio;
// public se incluye por si en BE-T4b se necesita una extensión compartida.
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
