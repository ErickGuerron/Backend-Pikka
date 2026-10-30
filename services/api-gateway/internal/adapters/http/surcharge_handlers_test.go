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
	listCalls       int
	resolveReq      *ordersv1.ResolveSurchargeVerificationRequest
	chargeReq       *ordersv1.ApplyRecidivismChargeRequest
	clearReq        *ordersv1.ClearRecidivistRequest
	clearCalls      int
	feeReq          *ordersv1.SetReassignmentFeeRequest
	reassignReq     *ordersv1.ApplyReassignmentChargeRequest
	reassignCalls   int
	recidivistCalls int
	payCalls        int
	payRes          *ordersv1.PayMyBalanceResponse
	meta            grpcclient.CallMeta
	err             error
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

func (f *fakeSurchargeClient) PayMyBalance(_ context.Context, m grpcclient.CallMeta, _ *ordersv1.PayMyBalanceRequest) (*ordersv1.PayMyBalanceResponse, error) {
	f.payCalls++
	f.meta = m
	if f.err != nil {
		return nil, f.err
	}
	if f.payRes != nil {
		return f.payRes, nil
	}
	return &ordersv1.PayMyBalanceResponse{Status: "SIMULATED_APPROVED", Amount: "7.50", Reference: "SIM-1", Unblocked: true, CreatedAt: "2026-10-29T09:00:00Z"}, nil
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

func (f *fakeSurchargeClient) ListRecidivists(_ context.Context, m grpcclient.CallMeta, _ *ordersv1.ListRecidivistsRequest) (*ordersv1.ListRecidivistsResponse, error) {
	f.recidivistCalls++
	f.meta = m
	if f.err != nil {
		return nil, f.err
	}
	return &ordersv1.ListRecidivistsResponse{Recidivists: []*ordersv1.Recidivist{
		{CustomerId: "c-1", FullName: "Ana Perez", ContactPhone: "0991234567", RecidivistAt: "2026-10-10T09:00:00Z", PendingSurcharge: "4.50", ChargeApplied: true},
	}}, nil
}

const recidivistsPath = "/api/v1/customers/recidivists"

func TestListRecidivists_StaffGetsAnArrayWithOnlyWhatTheyNeed(t *testing.T) {
	for _, role := range []string{"OPERATOR", "ADMIN"} {
		fake := &fakeSurchargeClient{}
		rec := do(surchargeRouter(fake), http.MethodGet, recidivistsPath, "", authHdr(t, role))

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.JSONEq(t, `[{"customerId":"c-1","fullName":"Ana Perez","contactPhone":"0991234567","recidivistAt":"2026-10-10T09:00:00Z","pendingSurcharge":"4.50","chargeApplied":true}]`, rec.Body.String())
		assert.Equal(t, role, fake.meta.Role)
	}
}

func TestListRecidivists_NoRecidivistsIsAnEmptyArrayNotNull(t *testing.T) {
	rec := do(surchargeRouter(&emptyRecidivists{}), http.MethodGet, recidivistsPath, "", authHdr(t, "OPERATOR"))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `[]`, rec.Body.String())
}

func TestListRecidivists_CustomerAndDriverAre403BeforeTheService(t *testing.T) {
	for _, role := range []string{"CUSTOMER", "DRIVER"} {
		fake := &fakeSurchargeClient{}
		rec := do(surchargeRouter(fake), http.MethodGet, recidivistsPath, "", authHdr(t, role))

		assert.Equal(t, http.StatusForbidden, rec.Code, role)
		assert.Equal(t, 0, fake.recidivistCalls, role)
	}
}

func TestListRecidivists_WithoutToken401(t *testing.T) {
	rec := do(surchargeRouter(&fakeSurchargeClient{}), http.MethodGet, recidivistsPath, "", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestListRecidivists_ServiceErrorKeepsItsStatus(t *testing.T) {
	fake := &fakeSurchargeClient{err: status.Error(codes.PermissionDenied, "no")}
	rec := do(surchargeRouter(fake), http.MethodGet, recidivistsPath, "", authHdr(t, "OPERATOR"))
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

type emptyRecidivists struct{ fakeSurchargeClient }

func (emptyRecidivists) ListRecidivists(context.Context, grpcclient.CallMeta, *ordersv1.ListRecidivistsRequest) (*ordersv1.ListRecidivistsResponse, error) {
	return &ordersv1.ListRecidivistsResponse{}, nil
}

func (f *fakeSurchargeClient) ClearRecidivist(_ context.Context, m grpcclient.CallMeta, req *ordersv1.ClearRecidivistRequest) (*ordersv1.ClearRecidivistResponse, error) {
	f.clearCalls++
	f.clearReq, f.meta = req, m
	if f.err != nil {
		return nil, f.err
	}
	return &ordersv1.ClearRecidivistResponse{Clear: &ordersv1.RecidivistClear{
		Id: "k-1", CustomerId: req.GetCustomerId(), RecidivistAt: "2026-10-10T09:00:00Z", Reason: req.GetReason(),
		ClearedBy: "u-1", CreatedAt: "2026-10-30T09:00:00Z",
	}}, nil
}

const clearPath = "/api/v1/customers/c-1/recidivist/clear"

func TestClearRecidivist_AdminForwardsTheReason(t *testing.T) {
	fake := &fakeSurchargeClient{}

	rec := do(surchargeRouter(fake), http.MethodPost, clearPath, `{"reason":"Pagó todo y se acordó con él"}`, authHdr(t, "ADMIN"))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "c-1", fake.clearReq.GetCustomerId())
	assert.Equal(t, "Pagó todo y se acordó con él", fake.clearReq.GetReason())
	assert.Equal(t, "ADMIN", fake.meta.Role)
	assert.JSONEq(t, `{"id":"k-1","customerId":"c-1","recidivistAt":"2026-10-10T09:00:00Z","reason":"Pagó todo y se acordó con él","clearedBy":"u-1","createdAt":"2026-10-30T09:00:00Z"}`, rec.Body.String())
}

func TestClearRecidivist_OperatorCustomerAndDriverAre403BeforeTheService(t *testing.T) {
	for _, role := range []string{"OPERATOR", "CUSTOMER", "DRIVER"} {
		fake := &fakeSurchargeClient{}
		rec := do(surchargeRouter(fake), http.MethodPost, clearPath, `{"reason":"Pagó todo y se acordó con él"}`, authHdr(t, role))
		assert.Equal(t, http.StatusForbidden, rec.Code, role)
		assert.Equal(t, 0, fake.clearCalls, role)
	}
}

func TestClearRecidivist_WithoutToken401(t *testing.T) {
	rec := do(surchargeRouter(&fakeSurchargeClient{}), http.MethodPost, clearPath, `{"reason":"Pagó todo y se acordó con él"}`, nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestClearRecidivist_ServiceErrorsKeepTheirStatus(t *testing.T) {
	cases := map[codes.Code]int{codes.FailedPrecondition: http.StatusUnprocessableEntity, codes.NotFound: http.StatusNotFound, codes.InvalidArgument: http.StatusBadRequest}
	for code, want := range cases {
		fake := &fakeSurchargeClient{err: status.Error(code, "x")}
		rec := do(surchargeRouter(fake), http.MethodPost, clearPath, `{"reason":"Pagó todo y se acordó con él"}`, authHdr(t, "ADMIN"))
		assert.Equal(t, want, rec.Code, code.String())
	}
}

const payPath = "/api/v1/customers/me/balance/payment"

// ADR 0006 seccion 4: el cobro se marca como simulado en la interfaz, para que nadie crea que hubo un cargo real.
func TestPayMyBalance_CustomerPaysAndTheAnswerSaysItIsSimulated(t *testing.T) {
	fake := &fakeSurchargeClient{}

	rec := do(surchargeRouter(fake), http.MethodPost, payPath, "", authHdr(t, "CUSTOMER"))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"status":"SIMULATED_APPROVED","simulated":true,"amount":"7.50","reference":"SIM-1","unblocked":true,"createdAt":"2026-10-29T09:00:00Z"}`, rec.Body.String())
	assert.Equal(t, "CUSTOMER", fake.meta.Role)
	assert.Equal(t, 1, fake.payCalls)
}

func TestPayMyBalance_DeclinedCardIsAnswered200WithItsStatus(t *testing.T) {
	fake := &fakeSurchargeClient{payRes: &ordersv1.PayMyBalanceResponse{Status: "SIMULATED_DECLINED", Amount: "7.99", Reference: "SIM-2", CreatedAt: "2026-10-29T09:00:00Z"}}

	rec := do(surchargeRouter(fake), http.MethodPost, payPath, "", authHdr(t, "CUSTOMER"))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"status":"SIMULATED_DECLINED"`)
	assert.Contains(t, rec.Body.String(), `"unblocked":false`)
	assert.Contains(t, rec.Body.String(), `"simulated":true`)
}

func TestPayMyBalance_OnlyTheCustomerRoleReachesTheService(t *testing.T) {
	for _, role := range []string{"OPERATOR", "ADMIN", "DRIVER"} {
		fake := &fakeSurchargeClient{}
		rec := do(surchargeRouter(fake), http.MethodPost, payPath, "", authHdr(t, role))

		assert.Equal(t, http.StatusForbidden, rec.Code, role)
		assert.Equal(t, 0, fake.payCalls, role)
	}
	rec := do(surchargeRouter(&fakeSurchargeClient{}), http.MethodPost, payPath, "", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestPayMyBalance_ServiceDecisionsKeepTheirStatus(t *testing.T) {
	cases := map[codes.Code]int{
		codes.FailedPrecondition: http.StatusUnprocessableEntity,
		codes.Aborted:            http.StatusConflict,
	}
	for code, want := range cases {
		rec := do(surchargeRouter(&fakeSurchargeClient{err: status.Error(code, "x")}), http.MethodPost, payPath, "", authHdr(t, "CUSTOMER"))
		assert.Equal(t, want, rec.Code, code.String())
	}
}

// El cuerpo no decide nada: el cliente no manda monto, solo se paga el saldo completo.
func TestPayMyBalance_IgnoresAnyBody(t *testing.T) {
	fake := &fakeSurchargeClient{}
	rec := do(surchargeRouter(fake), http.MethodPost, payPath, `{"amount":"0.01"}`, authHdr(t, "CUSTOMER"))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, 1, fake.payCalls)
}
