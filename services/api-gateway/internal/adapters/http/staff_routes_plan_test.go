package http_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
)

// Plan: a CUSTOMER token must never reach staff-only routes (BOLA / privilege escalation).
func TestPlan_CustomerCannotCallStaffRoutes(t *testing.T) {
	routing, orders, drivers, auth := &fakeRouting{}, &fakeOrderClient{}, &fakeDriverClient{}, &fakeAuth{}
	h := newRouterWithSignup(auth, httpapi.Options{Routing: routing, Orders: orders, Drivers: drivers})

	for _, tc := range []struct{ name, method, path, body string }{
		{"plan routes", http.MethodPost, "/api/v1/routes/plan", `{"deliveryDate":"2026-10-16"}`},
		{"staff order list", http.MethodGet, "/api/v1/orders", ""},
		{"create driver", http.MethodPost, "/api/v1/drivers", createDriverBody},
		{"list drivers", http.MethodGet, "/api/v1/drivers", ""},
		{"list users", http.MethodGet, "/api/v1/users", ""},
	} {
		rec := do(h, tc.method, tc.path, tc.body, authHdr(t, "CUSTOMER"))

		assert.Equal(t, http.StatusForbidden, rec.Code, tc.name)
	}
	assert.Zero(t, routing.calls)
	assert.Zero(t, auth.createCalls)
}
