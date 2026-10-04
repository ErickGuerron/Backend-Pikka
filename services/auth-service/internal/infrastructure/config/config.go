// Package config carga la configuración desde variables de entorno.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

const minJWTSecretLen = 32

// Config es la configuración del Auth Service.
type Config struct {
	AppEnv   string
	HTTPPort int
	GRPCPort int

	Postgres Postgres

	JWTSecret string
	JWTIssuer string
	JWTTTL    time.Duration

	BcryptCost int

	BootstrapAdminEmail    string
	BootstrapAdminPassword string
}

// Postgres agrupa los datos de conexión. Cada servicio usa su propio usuario y esquema.
type Postgres struct {
	Host     string
	Port     int
	DB       string
	User     string
	Password string
	Schema   string
	SSLMode  string
}

// DSN construye la URL de conexión fijando el search_path al esquema del servicio.
func (p Postgres) DSN() string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(p.User, p.Password),
		Host:   fmt.Sprintf("%s:%d", p.Host, p.Port),
		Path:   p.DB,
	}
	q := url.Values{}
	q.Set("sslmode", p.SSLMode)
	q.Set("search_path", p.Schema)
	u.RawQuery = q.Encode()
	return u.String()
}

// Load lee y valida la configuración.
func Load() (Config, error) {
	var errs []error
	cfg := Config{
		AppEnv:   getEnv("APP_ENV", "development"),
		HTTPPort: getInt("HTTP_PORT", 8080, &errs),
		GRPCPort: getInt("GRPC_PORT", 9090, &errs),
		Postgres: Postgres{
			Host:     getEnv("POSTGRES_HOST", "localhost"),
			Port:     getInt("POSTGRES_PORT", 5432, &errs),
			DB:       getEnv("POSTGRES_DB", "last_mile"),
			User:     getEnv("POSTGRES_USER", "auth_service_user"),
			Password: os.Getenv("POSTGRES_PASSWORD"),
			Schema:   getEnv("POSTGRES_SCHEMA", "auth"),
			SSLMode:  getEnv("POSTGRES_SSLMODE", "disable"),
		},
		JWTSecret:              os.Getenv("JWT_SECRET"),
		JWTIssuer:              getEnv("JWT_ISSUER", "lastmile-auth"),
		JWTTTL:                 getDuration("JWT_TTL", time.Hour, &errs),
		BcryptCost:             getInt("BCRYPT_COST", 12, &errs),
		BootstrapAdminEmail:    os.Getenv("BOOTSTRAP_ADMIN_EMAIL"),
		BootstrapAdminPassword: os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"),
	}

	if cfg.Postgres.Password == "" {
		errs = append(errs, errors.New("POSTGRES_PASSWORD is required"))
	}
	if len(cfg.JWTSecret) < minJWTSecretLen {
		errs = append(errs, fmt.Errorf("JWT_SECRET must be at least %d characters", minJWTSecretLen))
	}
	if cfg.JWTTTL <= 0 {
		errs = append(errs, errors.New("JWT_TTL must be positive"))
	}
	if (cfg.BootstrapAdminEmail == "") != (cfg.BootstrapAdminPassword == "") {
		errs = append(errs, errors.New("BOOTSTRAP_ADMIN_EMAIL and BOOTSTRAP_ADMIN_PASSWORD must be set together"))
	}
	return cfg, errors.Join(errs...)
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
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

func getDuration(key string, def time.Duration, errs *[]error) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s must be a duration like 1h or 15m", key))
		return def
	}
	return d
}
