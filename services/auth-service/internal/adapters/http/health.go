// Package http expone /health y /ready del servicio.
package http

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// ReadinessCheck verifica una dependencia (por ejemplo, PostgreSQL).
type ReadinessCheck func(ctx context.Context) error

// NewHealthServer devuelve un servidor HTTP con /health (proceso vivo) y /ready (dependencias listas).
// La direccion de escucha la fija quien lo arranca.
func NewHealthServer(ready ReadinessCheck) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, "ok")
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := ready(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, "unavailable")
			return
		}
		writeJSON(w, http.StatusOK, "ready")
	})
	return &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
}

// writeJSON responde {"status": s} con el mismo formato que el servicio usaba antes.
func writeJSON(w http.ResponseWriter, code int, status string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": status})
}