package httpx

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Header names and media types used by the gateway.
const (
	HeaderAuthorization   = "Authorization"
	HeaderContentType     = "Content-Type"
	HeaderXRequestID      = "X-Request-ID"
	HeaderOrigin          = "Origin"
	HeaderVary            = "Vary"
	HeaderXForwardedFor   = "X-Forwarded-For"
	HeaderXForwardedProto = "X-Forwarded-Proto"
	MIMEApplicationJSON   = "application/json"
)

// HTTPError is an error that carries the HTTP status to answer with.
type HTTPError struct {
	Code     int
	Message  any
	Internal error
}

// Error implements error.
func (e *HTTPError) Error() string { return fmt.Sprintf("code=%d, message=%v", e.Code, e.Message) }

// Unwrap exposes the internal error, if any.
func (e *HTTPError) Unwrap() error { return e.Internal }

// NewHTTPError builds an HTTPError. Without a message the standard status text is used.
func NewHTTPError(code int, message ...any) *HTTPError {
	he := &HTTPError{Code: code, Message: http.StatusText(code)}
	if len(message) > 0 {
		he.Message = message[0]
	}
	return he
}

// ErrNotFound is returned when no route matches the request.
var ErrNotFound = NewHTTPError(http.StatusNotFound)

// BindBody decodes a JSON request body into dst. An empty body (Content-Length 0) leaves dst untouched.
func BindBody(c *Context, dst any) error {
	req := c.Request()
	if req.ContentLength == 0 {
		return nil
	}
	return json.NewDecoder(req.Body).Decode(dst)
}
