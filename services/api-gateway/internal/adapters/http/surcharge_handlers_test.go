package http_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
)

type fakeSurchargeClient struct {
	listCalls  int
	resolveReq *ordersv1.ResolveSurchargeVerificationRequest
	chargeReq  *ordersv1.ApplyRecidivismChargeRequest
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

func (f *fakeSurchargeClient) ApplyRecidivismCharge(_ context.Context, m grpcclient.CallMeta, req *ordersv1.ApplyRecidivismChargeRequest) (*ordersv1.ApplyRecidivismChargeResponse, error) {
	f.chargeReq, f.meta = req, m
	if f.err != nil {
		return nil, f.err
	}
	return &ordersv1.ApplyRecidivismChargeResponse{Charge: &ordersv1.RecidivismCharge{
		Id: "r-1", CustomerId: req.GetCustomerId(), RecidivistAt: "2026-10-10T09:00:00Z", Amount: req.GetAmount(),
		Reason: req.GetReason(), AppliedBy: "u-1", CreatedAt: "2026-10-11T09:00:00Z",
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

const chargePath = "/api/v1/customers/c-1/recidivism-charge"

func TestApplyRecidivismCharge_StaffForwardsAmountAndReason(t *testing.T) {
	for _, role := range []string{"OPERATOR", "ADMIN"} {
		fake := &fakeSurchargeClient{}
		body := `{"amount":"12.50","reason":"Tres no entregas en dos semanas"}`

		rec := do(surchargeRouter(fake), http.MethodPost, chargePath, body, authHdr(t, role))

		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
		assert.Equal(t, "c-1", fake.chargeReq.GetCustomerId())
		assert.Equal(t, "12.50", fake.chargeReq.GetAmount())
		assert.Equal(t, "Tres no entregas en dos semanas", fake.chargeReq.GetReason())
		assert.Equal(t, role, fake.meta.Role)
		assert.JSONEq(t, `{"id":"r-1","customerId":"c-1","recidivistAt":"2026-10-10T09:00:00Z","amount":"12.50","reason":"Tres no entregas en dos semanas","appliedBy":"u-1","createdAt":"2026-10-11T09:00:00Z"}`, rec.Body.String())
	}
}

func TestApplyRecidivismCharge_CustomerAndDriverAre403BeforeTheService(t *testing.T) {
	for _, role := range []string{"CUSTOMER", "DRIVER"} {
		fake := &fakeSurchargeClient{}
		rec := do(surchargeRouter(fake), http.MethodPost, chargePath, `{"amount":"5","reason":"motivo valido"}`, authHdr(t, role))

		assert.Equal(t, http.StatusForbidden, rec.Code, role)
		assert.Nil(t, fake.chargeReq, role)
	}
}

func TestApplyRecidivismCharge_WithoutToken401(t *testing.T) {
	rec := do(surchargeRouter(&fakeSurchargeClient{}), http.MethodPost, chargePath, `{"amount":"5","reason":"motivo valido"}`, nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// El dinero viaja como texto decimal, como pendingSurcharge: un numero JSON es un error de entrada.
func TestApplyRecidivismCharge_NumericAmountOrBrokenBodyIs400(t *testing.T) {
	for _, body := range []string{`{"amount":12.5,"reason":"motivo valido"}`, `{"amount":`} {
		fake := &fakeSurchargeClient{}
		rec := do(surchargeRouter(fake), http.MethodPost, chargePath, body, authHdr(t, "OPERATOR"))

		assert.Equal(t, http.StatusBadRequest, rec.Code, body)
		assert.Nil(t, fake.chargeReq, body)
	}
}

func TestApplyRecidivismCharge_ServiceDecisionsKeepTheirStatus(t *testing.T) {
	cases := map[codes.Code]int{
		codes.FailedPrecondition: http.StatusUnprocessableEntity,
		codes.Aborted:            http.StatusConflict,
		codes.NotFound:           http.StatusNotFound,
		codes.InvalidArgument:    http.StatusBadRequest,
	}
	for code, want := range cases {
		fake := &fakeSurchargeClient{err: status.Error(code, "x")}
		rec := do(surchargeRouter(fake), http.MethodPost, chargePath, `{"amount":"5","reason":"motivo valido"}`, authHdr(t, "OPERATOR"))
		assert.Equal(t, want, rec.Code, code.String())
	}
}
