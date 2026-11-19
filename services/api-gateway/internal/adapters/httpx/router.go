package httpx

import (
	"errors"
	"net/http"
	"strings"
)

// ErrorHandlerFunc turns any error into a response. It must be safe to call after the response started.
type ErrorHandlerFunc func(err error, c *Context)

// Engine routes requests. Global middleware wraps routing, so it also sees unmatched requests.
type Engine struct {
	// HTTPErrorHandler writes the response for errors returned by handlers and middleware.
	HTTPErrorHandler ErrorHandlerFunc
	// IPExtractor decides the client IP used by RealIP. Nil falls back to the direct peer.
	IPExtractor func(*http.Request) string

	root       *Group
	middleware []MiddlewareFunc
	routes     []*route
}

type route struct {
	method  string
	pattern string
	segs    []string
	chain   HandlerFunc
}

// Group registers routes under a prefix with middleware applied to each of them.
type Group struct {
	engine     *Engine
	prefix     string
	middleware []MiddlewareFunc
}

// New returns an engine with no middleware and a plain-text default error handler.
func New() *Engine {
	e := &Engine{
		HTTPErrorHandler: func(err error, c *Context) {
			if c.res.Committed {
				return
			}
			code := http.StatusInternalServerError
			var he *HTTPError
			if errors.As(err, &he) {
				code = he.Code
			}
			_ = c.NoContent(code)
		},
		IPExtractor: ExtractIPDirect(),
	}
	e.root = &Group{engine: e}
	return e
}

// Use adds global middleware. It runs for every request, including unmatched ones.
func (e *Engine) Use(mw ...MiddlewareFunc) { e.middleware = append(e.middleware, mw...) }

// Group creates a sub-group. Its middleware runs after the parent group's middleware.
func (e *Engine) Group(prefix string, mw ...MiddlewareFunc) *Group {
	return e.root.Group(prefix, mw...)
}

// GET registers a GET route at the root.
func (e *Engine) GET(path string, h HandlerFunc, mw ...MiddlewareFunc) { e.root.GET(path, h, mw...) }

// POST registers a POST route at the root.
func (e *Engine) POST(path string, h HandlerFunc, mw ...MiddlewareFunc) { e.root.POST(path, h, mw...) }

// PUT registers a PUT route at the root.
func (e *Engine) PUT(path string, h HandlerFunc, mw ...MiddlewareFunc) { e.root.PUT(path, h, mw...) }

// PATCH registers a PATCH route at the root.
func (e *Engine) PATCH(path string, h HandlerFunc, mw ...MiddlewareFunc) {
	e.root.PATCH(path, h, mw...)
}

// DELETE registers a DELETE route at the root.
func (e *Engine) DELETE(path string, h HandlerFunc, mw ...MiddlewareFunc) {
	e.root.DELETE(path, h, mw...)
}

// Group creates a sub-group with an extra prefix and middleware.
func (g *Group) Group(prefix string, mw ...MiddlewareFunc) *Group {
	all := make([]MiddlewareFunc, 0, len(g.middleware)+len(mw))
	all = append(all, g.middleware...)
	all = append(all, mw...)
	return &Group{engine: g.engine, prefix: g.prefix + prefix, middleware: all}
}

// GET registers a GET route in the group.
func (g *Group) GET(path string, h HandlerFunc, mw ...MiddlewareFunc) {
	g.add(http.MethodGet, path, h, mw...)
}

// POST registers a POST route in the group.
func (g *Group) POST(path string, h HandlerFunc, mw ...MiddlewareFunc) {
	g.add(http.MethodPost, path, h, mw...)
}

// PUT registers a PUT route in the group.
func (g *Group) PUT(path string, h HandlerFunc, mw ...MiddlewareFunc) {
	g.add(http.MethodPut, path, h, mw...)
}

// PATCH registers a PATCH route in the group.
func (g *Group) PATCH(path string, h HandlerFunc, mw ...MiddlewareFunc) {
	g.add(http.MethodPatch, path, h, mw...)
}

// DELETE registers a DELETE route in the group.
func (g *Group) DELETE(path string, h HandlerFunc, mw ...MiddlewareFunc) {
	g.add(http.MethodDelete, path, h, mw...)
}

func (g *Group) add(method, path string, h HandlerFunc, mw ...MiddlewareFunc) {
	full := g.prefix + path
	chain := h
	all := make([]MiddlewareFunc, 0, len(g.middleware)+len(mw))
	all = append(all, g.middleware...)
	all = append(all, mw...)
	for i := len(all) - 1; i >= 0; i-- {
		chain = all[i](chain)
	}
	g.engine.routes = append(g.engine.routes, &route{
		method:  method,
		pattern: full,
		segs:    strings.Split(full, "/"),
		chain:   chain,
	})
}

// ServeHTTP implements http.Handler.
func (e *Engine) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := NewContext(e, w, r)
	var h HandlerFunc = e.dispatch
	for i := len(e.middleware) - 1; i >= 0; i-- {
		h = e.middleware[i](h)
	}
	if err := h(c); err != nil {
		c.Error(err)
	}
}

// dispatch selects the route. A path that exists under another method is reported as 404, which is
// the gateway's documented contract (ServeMux would answer 405 instead).
func (e *Engine) dispatch(c *Context) error {
	rt, vals := e.match(c.req)
	if rt == nil {
		return ErrNotFound
	}
	c.pattern = rt.pattern
	c.paramNames = c.paramNames[:0]
	c.paramValues = vals
	for _, s := range rt.segs {
		if strings.HasPrefix(s, ":") {
			c.paramNames = append(c.paramNames, s[1:])
		}
	}
	return rt.chain(c)
}

// match returns the best route for the request. A static segment beats a parameter segment at the
// first position where they differ; ties go to the route registered first.
func (e *Engine) match(r *http.Request) (*route, []string) {
	segs := strings.Split(r.URL.Path, "/")
	var best *route
	var bestVals []string
	var bestScore []int
	for _, rt := range e.routes {
		if rt.method != r.Method || len(rt.segs) != len(segs) {
			continue
		}
		vals := make([]string, 0, 2)
		score := make([]int, len(segs))
		ok := true
		for i, want := range rt.segs {
			if strings.HasPrefix(want, ":") {
				if segs[i] == "" {
					ok = false
					break
				}
				vals = append(vals, segs[i])
				score[i] = 1
				continue
			}
			if want != segs[i] {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		if best == nil || lessScore(score, bestScore) {
			best, bestVals, bestScore = rt, vals, score
		}
	}
	return best, bestVals
}

func lessScore(a, b []int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}
