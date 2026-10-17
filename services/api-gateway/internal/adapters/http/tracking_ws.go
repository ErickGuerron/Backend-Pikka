package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/labstack/echo/v4"

	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
)

// trackingSubprotocol es el protocolo que el cliente pide. El token viaja como segundo subprotocolo
// (Sec-WebSocket-Protocol: pikka.v1, <jwt>): los navegadores no pueden poner Authorization en un socket,
// y el token nunca va en la URL.
const trackingSubprotocol = "pikka.v1"

// isWebSocketUpgrade indica si la petición pide actualizar a WebSocket.
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// tokenFromSubprotocols extrae el JWT de la segunda posición de Sec-WebSocket-Protocol.
// Cualquier otra forma se rechaza igual que un token ausente.
func tokenFromSubprotocols(values []string) (string, error) {
	parts := strings.Split(strings.Join(values, ","), ",")
	if len(parts) < 2 || strings.TrimSpace(parts[0]) != trackingSubprotocol {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "missing bearer subprotocol")
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", echo.NewHTTPError(http.StatusUnauthorized, "missing bearer subprotocol")
	}
	return token, nil
}

// ws valida identidad y pertenencia antes de actualizar la conexión. Un pedido ajeno o inexistente
// responde 404 por HTTP, sin abrir socket. Después envía el snapshot y sondea hasta que el cliente se va.
func (h *customerTrackingHandlers) ws(c echo.Context) error {
	token, err := tokenFromSubprotocols(c.Request().Header.Values("Sec-WebSocket-Protocol"))
	if err != nil {
		return err
	}
	id, err := h.verifier.Verify(token)
	if err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid or expired token")
	}
	if !strings.EqualFold(id.Role, "CUSTOMER") {
		return echo.NewHTTPError(http.StatusForbidden, "customer role required")
	}

	m := grpcclient.CallMeta{RequestID: c.Response().Header().Get(echo.HeaderXRequestID), UserID: id.UserID, Role: id.Role}
	orderID := c.Param("id")
	ctx := c.Request().Context()
	first, err := h.build(ctx, m, orderID)
	if err != nil {
		return err
	}

	// El servidor puede tener ReadTimeout/WriteTimeout activos; el socket los heredaría tras el hijack.
	// Se quitan antes de actualizar: la vida del socket la gobiernan el cliente y el ticker.
	rc := http.NewResponseController(c.Response())
	_ = rc.SetReadDeadline(time.Time{})
	_ = rc.SetWriteDeadline(time.Time{})

	conn, err := websocket.Accept(c.Response(), c.Request(), &websocket.AcceptOptions{
		Subprotocols:   []string{trackingSubprotocol},
		OriginPatterns: h.origins,
	})
	if err != nil {
		// Accept ya escribió la respuesta HTTP de rechazo.
		return nil
	}
	// Best-effort: by this point the socket is already closed or the peer is gone.
	defer func() { _ = conn.CloseNow() }()
	return h.push(ctx, conn, m, orderID, first)
}

// push envía el snapshot inicial y, en cada sondeo, otro solo si cambió. Se ejecuta en una sola goroutine,
// así que los sondeos llegan en orden y un duplicado nunca se adelanta a un cambio.
func (h *customerTrackingHandlers) push(ctx context.Context, conn *websocket.Conn, m grpcclient.CallMeta, orderID string, last trackingSnapshot) error {
	// CloseRead descarta lo que manda el cliente y cancela ctx cuando el cliente se va.
	ctx = conn.CloseRead(ctx)
	if err := writeSnapshot(ctx, conn, last); err != nil {
		return nil
	}
	ticks, stop := h.ticker(h.interval)
	defer stop()
	for {
		select {
		case <-ctx.Done():
			_ = conn.Close(websocket.StatusNormalClosure, "")
			return nil
		case <-ticks:
			next, err := h.build(ctx, m, orderID)
			if err != nil || sameView(last, next) {
				continue
			}
			if err := writeSnapshot(ctx, conn, next); err != nil {
				return nil
			}
			last = next
		}
	}
}

func writeSnapshot(ctx context.Context, conn *websocket.Conn, snap trackingSnapshot) error {
	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, data)
}
