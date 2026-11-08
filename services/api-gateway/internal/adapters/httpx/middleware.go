package httpx

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Recover turns a panic in the rest of the chain into an error, which the error handler answers
// with a JSON 500.
func Recover() MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("panic: %v", r)
				}
			}()
			return next(c)
		}
	}
}

// SecureConfig selects the security headers. Empty strings and zero values are not sent.
type SecureConfig struct {
	XSSProtection         string
	ContentTypeNosniff    string
	XFrameOptions         string
	HSTSMaxAge            int
	ContentSecurityPolicy string
	ReferrerPolicy        string
}

// Secure sets security headers. HSTS is only sent when the request arrived over HTTPS.
func Secure(cfg SecureConfig) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			h := c.Response().Header()
			if cfg.XSSProtection != "" {
				h.Set("X-XSS-Protection", cfg.XSSProtection)
			}
			if cfg.ContentTypeNosniff != "" {
				h.Set("X-Content-Type-Options", cfg.ContentTypeNosniff)
			}
			if cfg.XFrameOptions != "" {
				h.Set("X-Frame-Options", cfg.XFrameOptions)
			}
			if cfg.HSTSMaxAge != 0 && c.Scheme() == "https" {
				h.Set("Strict-Transport-Security", fmt.Sprintf("max-age=%d; includeSubdomains", cfg.HSTSMaxAge))
			}
			if cfg.ContentSecurityPolicy != "" {
				h.Set("Content-Security-Policy", cfg.ContentSecurityPolicy)
			}
			if cfg.ReferrerPolicy != "" {
				h.Set("Referrer-Policy", cfg.ReferrerPolicy)
			}
			return next(c)
		}
	}
}

// CORSConfig describes which cross-origin requests are allowed. Origins are compared exactly.
type CORSConfig struct {
	AllowOrigins  []string
	AllowMethods  []string
	AllowHeaders  []string
	ExposeHeaders []string
	MaxAge        int
}

// CORS answers preflight requests and adds the Access-Control headers for allowed origins.
// Requests from other origins are passed through without CORS headers, as the browser enforces them.
func CORS(cfg CORSConfig) MiddlewareFunc {
	expose := make([]string, 0, len(cfg.ExposeHeaders))
	for _, h := range cfg.ExposeHeaders {
		expose = append(expose, http.CanonicalHeaderKey(h))
	}
	allowMethods := strings.Join(cfg.AllowMethods, ",")
	allowHeaders := strings.Join(cfg.AllowHeaders, ",")
	exposeHeaders := strings.Join(expose, ",")
	maxAge := "0"
	if cfg.MaxAge > 0 {
		maxAge = strconv.Itoa(cfg.MaxAge)
	}
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			req := c.Request()
			res := c.Response()
			origin := req.Header.Get(HeaderOrigin)
			preflight := req.Method == http.MethodOptions
			res.Header().Add(HeaderVary, HeaderOrigin)

			if origin == "" {
				if !preflight {
					return next(c)
				}
				return c.NoContent(http.StatusNoContent)
			}

			allowOrigin := ""
			for _, o := range cfg.AllowOrigins {
				if o == "*" || o == origin {
					allowOrigin = o
					break
				}
			}
			if allowOrigin == "" {
				if !preflight {
					return next(c)
				}
				return c.NoContent(http.StatusNoContent)
			}

			res.Header().Set("Access-Control-Allow-Origin", allowOrigin)
			if !preflight {
				if exposeHeaders != "" {
					res.Header().Set("Access-Control-Expose-Headers", exposeHeaders)
				}
				return next(c)
			}

			res.Header().Add(HeaderVary, "Access-Control-Request-Method")
			res.Header().Add(HeaderVary, "Access-Control-Request-Headers")
			res.Header().Set("Access-Control-Allow-Methods", allowMethods)
			if allowHeaders != "" {
				res.Header().Set("Access-Control-Allow-Headers", allowHeaders)
			} else if h := req.Header.Get("Access-Control-Request-Headers"); h != "" {
				res.Header().Set("Access-Control-Allow-Headers", h)
			}
			res.Header().Set("Access-Control-Max-Age", maxAge)
			return c.NoContent(http.StatusNoContent)
		}
	}
}

// BodyLimit rejects requests whose declared Content-Length exceeds limit with 413, and caps how many
// bytes a handler can read. limit accepts a byte count with an optional K, M or G suffix ("1K").
func BodyLimit(limit string) MiddlewareFunc {
	limitBytes, err := parseByteSize(limit)
	if err != nil {
		panic(err)
	}
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			req := c.Request()
			if req.ContentLength > limitBytes {
				return NewHTTPError(http.StatusRequestEntityTooLarge)
			}
			req.Body = http.MaxBytesReader(c.Response(), req.Body, limitBytes)
			return next(c)
		}
	}
}

func parseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	s = strings.TrimSuffix(s, "B")
	mult := int64(1)
	switch {
	case strings.HasSuffix(s, "K"):
		mult, s = 1<<10, strings.TrimSuffix(s, "K")
	case strings.HasSuffix(s, "M"):
		mult, s = 1<<20, strings.TrimSuffix(s, "M")
	case strings.HasSuffix(s, "G"):
		mult, s = 1<<30, strings.TrimSuffix(s, "G")
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("httpx: invalid body limit %q", s)
	}
	return n * mult, nil
}

// MemoryRateLimiter keeps one token bucket per identifier in process memory.
type MemoryRateLimiter struct {
	mu          sync.Mutex
	rate        rate.Limit
	burst       int
	expiresIn   time.Duration
	visitors    map[string]*visitor
	lastCleanup time.Time
}

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewMemoryRateLimiter returns a limiter that allows rate events per second with the given burst.
// Buckets idle for longer than expiresIn are discarded.
func NewMemoryRateLimiter(r rate.Limit, burst int, expiresIn time.Duration) *MemoryRateLimiter {
	return &MemoryRateLimiter{rate: r, burst: burst, expiresIn: expiresIn, visitors: map[string]*visitor{}, lastCleanup: time.Now()}
}

func (s *MemoryRateLimiter) allow(id string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.Sub(s.lastCleanup) > s.expiresIn {
		for k, v := range s.visitors {
			if now.Sub(v.lastSeen) > s.expiresIn {
				delete(s.visitors, k)
			}
		}
		s.lastCleanup = now
	}
	v, ok := s.visitors[id]
	if !ok {
		v = &visitor{limiter: rate.NewLimiter(s.rate, s.burst)}
		s.visitors[id] = v
	}
	v.lastSeen = now
	return v.limiter.Allow()
}

// RateLimiter answers 429 when the client identified by RealIP exceeds the store's budget.
func RateLimiter(store *MemoryRateLimiter) MiddlewareFunc {
	return func(next HandlerFunc) HandlerFunc {
		return func(c *Context) error {
			if !store.allow(c.RealIP(), time.Now()) {
				return NewHTTPError(http.StatusTooManyRequests, "too many requests")
			}
			return next(c)
		}
	}
}
