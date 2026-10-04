// Package config carga la configuración del API Gateway desde variables de entorno.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const minJWTSecretLen = 32

// Config es la configuración del Gateway.
type Config struct {
	AppEnv   string
	HTTPPort int

	AuthServiceAddr string
	GRPCTimeout     time.Duration

	JWTSecret string
	JWTIssuer string

	CORSAllowedOrigins []string
	RateLimitPerSecond float64
	RateLimitBurst     int
	LoginRatePerMinute float64
	RequestTimeout     time.Duration
	BodyLimit          string
	// TrustProxy indica si se confía en X-Forwarded-For para identificar al cliente.
	TrustProxy bool
	// HSTS solo tiene sentido detrás de HTTPS (por ejemplo, en el balanceador).
	EnableHSTS bool
}

// Load lee y valida la configuración.
func Load() (Config, error) {
	var errs []error
	cfg := Config{
		AppEnv:             getEnv("APP_ENV", "development"),
		HTTPPort:           getInt("HTTP_PORT", 8080, &errs),
		AuthServiceAddr:    getEnv("AUTH_SERVICE_ADDR", "localhost:9090"),
		GRPCTimeout:        getDuration("GRPC_TIMEOUT", 3*time.Second, &errs),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		JWTIssuer:          getEnv("JWT_ISSUER", "lastmile-auth"),
		CORSAllowedOrigins: splitList(getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:5173")),
		RateLimitPerSecond: getFloat("RATE_LIMIT_PER_SECOND", 20, &errs),
		RateLimitBurst:     getInt("RATE_LIMIT_BURST", 40, &errs),
		LoginRatePerMinute: getFloat("LOGIN_RATE_PER_MINUTE", 10, &errs),
		RequestTimeout:     getDuration("REQUEST_TIMEOUT", 10*time.Second, &errs),
		BodyLimit:          getEnv("BODY_LIMIT", "1M"),
		TrustProxy:         getEnv("TRUST_PROXY", "false") == "true",
		EnableHSTS:         getEnv("ENABLE_HSTS", "true") == "true",
	}
	if len(cfg.JWTSecret) < minJWTSecretLen {
		errs = append(errs, fmt.Errorf("JWT_SECRET must be at least %d characters", minJWTSecretLen))
	}
	if cfg.GRPCTimeout <= 0 || cfg.RequestTimeout <= 0 {
		errs = append(errs, errors.New("GRPC_TIMEOUT and REQUEST_TIMEOUT must be positive"))
	}
	for _, o := range cfg.CORSAllowedOrigins {
		if o == "*" {
			errs = append(errs, errors.New("CORS_ALLOWED_ORIGINS must list explicit origins, not *"))
		}
	}
	return cfg, errors.Join(errs...)
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
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

func getFloat(key string, def float64, errs *[]error) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s must be a number", key))
		return def
	}
	return f
}

func getDuration(key string, def time.Duration, errs *[]error) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s must be a duration like 3s", key))
		return def
	}
	return d
}
