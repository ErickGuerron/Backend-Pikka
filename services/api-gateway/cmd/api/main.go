// Command api arranca el API Gateway: único punto de entrada REST/JSON del sistema.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/infrastructure/config"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/infrastructure/jwt"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/infrastructure/logging"
)

const serviceName = "api-gateway"

func main() {
	log := logging.New(serviceName, os.Getenv("LOG_LEVEL"))
	if err := run(log); err != nil {
		log.Error("service stopped with error", "error", err.Error())
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	authConn, err := grpcclient.Dial(cfg.AuthServiceAddr)
	if err != nil {
		return fmt.Errorf("dial auth service: %w", err)
	}
	defer func() { _ = authConn.Close() }()
	auth := grpcclient.NewAuth(authConn, cfg.GRPCTimeout)

	e := httpapi.NewRouter(log, httpapi.Options{
		CORSAllowedOrigins: cfg.CORSAllowedOrigins,
		RateLimitPerSecond: cfg.RateLimitPerSecond,
		RateLimitBurst:     cfg.RateLimitBurst,
		LoginRatePerMinute: cfg.LoginRatePerMinute,
		RequestTimeout:     cfg.RequestTimeout,
		BodyLimit:          cfg.BodyLimit,
		TrustProxy:         cfg.TrustProxy,
		EnableHSTS:         cfg.EnableHSTS,
	}, jwt.NewVerifier(cfg.JWTSecret, cfg.JWTIssuer), auth, auth.Ready)

	e.Server.ReadHeaderTimeout = 5 * time.Second
	e.Server.ReadTimeout = 15 * time.Second
	e.Server.WriteTimeout = cfg.RequestTimeout + 5*time.Second
	e.Server.IdleTimeout = 60 * time.Second

	errCh := make(chan error, 1)
	go func() {
		log.Info("http listening", "port", cfg.HTTPPort)
		if err := e.Start(fmt.Sprintf(":%d", cfg.HTTPPort)); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		log.Info("shutting down")
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return e.Shutdown(shutdownCtx)
}
