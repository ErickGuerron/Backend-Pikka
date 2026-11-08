// Package httpx is a small, dependency-free HTTP layer built on net/http. It offers the handful of
// primitives the gateway uses (context, router with groups, middleware, error mapping) so the gateway
// does not depend on a third-party web framework.
package httpx

import (
	"encoding/json"
	"net/http"
)

// HandlerFunc handles one request. Returning an error routes it through the engine's error handler.
type HandlerFunc func(*Context) error

// MiddlewareFunc wraps a HandlerFunc. The first middleware in a chain is the outermost.
type MiddlewareFunc func(HandlerFunc) HandlerFunc

// Response wraps http.ResponseWriter and records the status and whether headers were sent.
type Response struct {
	http.ResponseWriter
	Status    int
	Committed bool
}

func newResponse(w http.ResponseWriter) *Response {
	return &Response{ResponseWriter: w, Status: http.StatusOK}
}

// WriteHeader sends the status once; later calls are ignored, as net/http would log a superfluous call.
func (r *Response) WriteHeader(code int) {
	if r.Committed {
		return
	}
	r.Status = code
	r.Committed = true
	r.ResponseWriter.WriteHeader(code)
}

// Write commits a 200 status when no status was written first.
func (r *Response) Write(b []byte) (int, error) {
	if !r.Committed {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}

// Unwrap exposes the underlying writer so http.ResponseController can reach optional interfaces
// such as http.Hijacker (used by the WebSocket upgrade).
func (r *Response) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// Context carries one request through the middleware chain and the handler.
type Context struct {
	res         *Response
	req         *http.Request
	engine      *Engine
	pattern     string
	paramNames  []string
	paramValues []string
	store       map[string]any
}

// NewContext builds a context outside the router, for unit tests of handlers and middleware.
func NewContext(e *Engine, w http.ResponseWriter, r *http.Request) *Context {
	if e == nil {
		e = New()
	}
	return &Context{res: newResponse(w), req: r, engine: e, store: map[string]any{}}
}

// Request returns the current request.
func (c *Context) Request() *http.Request { return c.req }

// SetRequest replaces the current request (used by middleware that adds a context deadline).
func (c *Context) SetRequest(r *http.Request) { c.req = r }

// Response returns the response wrapper.
func (c *Context) Response() *Response { return c.res }

// Path returns the matched route pattern, such as /api/v1/orders/:id.
func (c *Context) Path() string { return c.pattern }

// Param returns a path parameter by name.
func (c *Context) Param(name string) string {
	for i, n := range c.paramNames {
		if n == name {
			return c.paramValues[i]
		}
	}
	return ""
}

// QueryParam returns the first query value for name.
func (c *Context) QueryParam(name string) string { return c.req.URL.Query().Get(name) }

// Set stores a value for the lifetime of the request.
func (c *Context) Set(key string, val any) {
	if c.store == nil {
		c.store = map[string]any{}
	}
	c.store[key] = val
}

// Get returns a value stored with Set.
func (c *Context) Get(key string) any { return c.store[key] }

// Error passes err to the engine's error handler, which writes the response.
func (c *Context) Error(err error) {
	if c.engine != nil && c.engine.HTTPErrorHandler != nil {
		c.engine.HTTPErrorHandler(err, c)
	}
}

// RealIP returns the client IP as decided by the engine's IPExtractor.
func (c *Context) RealIP() string {
	if c.engine != nil && c.engine.IPExtractor != nil {
		return c.engine.IPExtractor(c.req)
	}
	return ExtractIPDirect()(c.req)
}

// Scheme returns "https" or "http" for the client-facing connection.
func (c *Context) Scheme() string { return schemeOf(c.req) }

// JSON writes v as JSON with the given status.
func (c *Context) JSON(code int, v any) error {
	c.res.Header().Set("Content-Type", "application/json; charset=UTF-8")
	c.res.WriteHeader(code)
	return json.NewEncoder(c.res).Encode(v)
}

// Blob writes raw bytes with the given content type and status.
func (c *Context) Blob(code int, contentType string, b []byte) error {
	c.res.Header().Set("Content-Type", contentType)
	c.res.WriteHeader(code)
	_, err := c.res.Write(b)
	return err
}

// HTML writes an HTML document with the given status.
func (c *Context) HTML(code int, html string) error {
	return c.Blob(code, "text/html; charset=UTF-8", []byte(html))
}

// NoContent writes only the status line.
func (c *Context) NoContent(code int) error {
	c.res.WriteHeader(code)
	return nil
}
