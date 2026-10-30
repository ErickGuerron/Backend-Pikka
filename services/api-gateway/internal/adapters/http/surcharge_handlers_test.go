package http_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
)

type fakeSurchargeClient struct {
	listCalls  int
	resolveReq *ordersv1.ResolveSurchargeVerificationRequest
	meta       grpcclient.CallMeta
	err        error
}

func (f *fakeSurchargeClient) ListSurchargesPendingVerification(_ context.Context, m grpcclient.CallMeta, _ *ordersv1.ListSurchargesPendingVerificationRequest) (*ordersv1.ListSurchargesPendingVerificationResponse, error) {
	f.listCalls++
	f.meta = m
	if f.err != nil {
		return nil, f.err
	}
	return &ordersv1.ListSurchargesPendingVerificationResponse{Surcharges: []*ordersv1.Surcharge{
		{Id: "s-1", CustomerId: "c-1", OrderId: "o-1", Reason: "WRONG_ADDRESS", Status: "PENDING_VERIFICATION", CreatedAt: "2026-10-29T10:00:00Z"},
	}}, nil
}

func (f *fakeSurchargeClient) ResolveSurchargeVerification(_ context.Context, m grpcclient.CallMeta, req *ordersv1.ResolveSurchargeVerificationRequest) (*ordersv1.ResolveSurchargeVerificationResponse, error) {
	f.resolveReq, f.meta = req, m
	if f.err != nil {
		return nil, f.err
	}
	return &ordersv1.ResolveSurchargeVerificationResponse{Surcharge: &ordersv1.Surcharge{
		Id: req.GetId(), CustomerId: "c-1", OrderId: "o-1", Reason: "WRONG_ADDRESS", Status: "WAIVED",
		ResolvedBy: "u-1", ResolutionReason: req.GetReason(), ResolvedAt: "2026-10-29T11:00:00Z", CreatedAt: "2026-10-29T10:00:00Z",
	}}, nil
}

func surchargeRouter(c httpapi.SurchargeClient) http.Handler {
	return newRouterWithSignup(&fakeAuth{}, httpapi.Options{Surcharges: c})
}

const verifyPath = "/api/v1/surcharges/s-1/verification"

func TestListSurchargesPendingVerification_StaffGetsArray(t *testing.T) {
	for _, role := range []string{"OPERATOR", "ADMIN"} {
		fake := &fakeSurchargeClient{}
		rec := do(surchargeRouter(fake), http.MethodGet, "/api/v1/surcharges/pending-verification", "", authHdr(t, role))

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.JSONEq(t, `[{"id":"s-1","customerId":"c-1","orderId":"o-1","reason":"WRONG_ADDRESS","status":"PENDING_VERIFICATION","createdAt":"2026-10-29T10:00:00Z"}]`, rec.Body.String())
		assert.Equal(t, role, fake.meta.Role)
	}
}

func TestListSurchargesPendingVerification_CustomerAndDriverAre403BeforeTheService(t *testing.T) {
	for _, role := range []string{"CUSTOMER", "DRIVER"} {
		fake := &fakeSurchargeClient{}
		rec := do(surchargeRouter(fake), http.MethodGet, "/api/v1/surcharges/pending-verification", "", authHdr(t, role))

		assert.Equal(t, http.StatusForbidden, rec.Code, role)
		assert.Equal(t, 0, fake.listCalls, role)
	}
}

func TestListSurchargesPendingVerification_WithoutToken401(t *testing.T) {
	rec := do(surchargeRouter(&fakeSurchargeClient{}), http.MethodGet, "/api/v1/surcharges/pending-verification", "", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestResolveSurchargeVerification_ForwardsDecisionAndReason(t *testing.T) {
	fake := &fakeSurchargeClient{}
	body := `{"decision":"WAIVE","reason":"La direccion era del negocio"}`

	rec := do(surchargeRouter(fake), http.MethodPost, verifyPath, body, authHdr(t, "OPERATOR"))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "s-1", fake.resolveReq.GetId())
	assert.Equal(t, "WAIVE", fake.resolveReq.GetDecision())
	assert.Equal(t, "La direccion era del negocio", fake.resolveReq.GetReason())
	assert.JSONEq(t, `{"id":"s-1","customerId":"c-1","orderId":"o-1","reason":"WRONG_ADDRESS","status":"WAIVED","createdAt":"2026-10-29T10:00:00Z","resolvedBy":"u-1","resolutionReason":"La direccion era del negocio","resolvedAt":"2026-10-29T11:00:00Z"}`, rec.Body.String())
}

func TestResolveSurchargeVerification_CustomerIs403AndNothingIsCalled(t *testing.T) {
	fake := &fakeSurchargeClient{}
	rec := do(surchargeRouter(fake), http.MethodPost, verifyPath, `{"decision":"WAIVE","reason":"motivo valido"}`, authHdr(t, "CUSTOMER"))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Nil(t, fake.resolveReq)
}

func TestResolveSurchargeVerification_BadBodyIs400(t *testing.T) {
	fake := &fakeSurchargeClient{}
	rec := do(surchargeRouter(fake), http.MethodPost, verifyPath, `{"decision":`, authHdr(t, "OPERATOR"))

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Nil(t, fake.resolveReq)
}
