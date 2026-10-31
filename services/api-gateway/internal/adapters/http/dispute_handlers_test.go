package http_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
)

func openDisputeDTO(id, statusText string) *ordersv1.Dispute {
	return &ordersv1.Dispute{Id: id, SurchargeId: "s-1", CustomerId: "c-1", Reason: "Yo estaba en casa", Status: statusText, OpenedAt: "2026-10-30T12:00:00Z"}
}

func (f *fakeSurchargeClient) OpenDispute(_ context.Context, m grpcclient.CallMeta, req *ordersv1.OpenDisputeRequest) (*ordersv1.OpenDisputeResponse, error) {
	f.disputeCalls++
	f.disputeReq, f.meta = req, m
	if f.err != nil {
		return nil, f.err
	}
	return &ordersv1.OpenDisputeResponse{Dispute: openDisputeDTO("d-1", "OPEN")}, nil
}

func (f *fakeSurchargeClient) ListOpenDisputes(_ context.Context, m grpcclient.CallMeta, _ *ordersv1.ListOpenDisputesRequest) (*ordersv1.ListOpenDisputesResponse, error) {
	f.disputeCalls++
	f.meta = m
	if f.err != nil {
		return nil, f.err
	}
	return &ordersv1.ListOpenDisputesResponse{Disputes: []*ordersv1.Dispute{openDisputeDTO("d-1", "OPEN")}}, nil
}

func (f *fakeSurchargeClient) ResolveDispute(_ context.Context, m grpcclient.CallMeta, req *ordersv1.ResolveDisputeRequest) (*ordersv1.ResolveDisputeResponse, error) {
	f.disputeCalls++
	f.resolveDispReq, f.meta = req, m
	if f.err != nil {
		return nil, f.err
	}
	d := openDisputeDTO(req.GetId(), req.GetDecision())
	d.ResolvedBy, d.ResolutionReason, d.ResolvedAt = "u-1", req.GetReason(), "2026-10-30T13:00:00Z"
	return &ordersv1.ResolveDisputeResponse{Dispute: d}, nil
}

const (
	disputePath    = "/api/v1/surcharges/s-1/dispute"
	disputesPath   = "/api/v1/disputes"
	resolutionPath = "/api/v1/disputes/d-1/resolution"
)

func TestOpenDispute_CustomerSendsOnlyTheReasonAndGets201(t *testing.T) {
	fake := &fakeSurchargeClient{}

	rec := do(surchargeRouter(fake), http.MethodPost, disputePath, `{"reason":"Yo estaba en casa y nadie tocó","customerId":"otro"}`, authHdr(t, "CUSTOMER"))

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.Equal(t, "s-1", fake.disputeReq.GetSurchargeId())
	assert.Equal(t, "Yo estaba en casa y nadie tocó", fake.disputeReq.GetReason())
	assert.Equal(t, "CUSTOMER", fake.meta.Role)
	assert.JSONEq(t, `{"id":"d-1","surchargeId":"s-1","customerId":"c-1","reason":"Yo estaba en casa","status":"OPEN","openedAt":"2026-10-30T12:00:00Z"}`, rec.Body.String())
}

func TestOpenDispute_OnlyACustomerReachesTheService(t *testing.T) {
	for _, role := range []string{"OPERATOR", "ADMIN", "DRIVER"} {
		fake := &fakeSurchargeClient{}
		rec := do(surchargeRouter(fake), http.MethodPost, disputePath, `{"reason":"Yo estaba en casa y nadie tocó"}`, authHdr(t, role))
		assert.Equal(t, http.StatusForbidden, rec.Code, role)
		assert.Equal(t, 0, fake.disputeCalls, role)
	}
	assert.Equal(t, http.StatusUnauthorized, do(surchargeRouter(&fakeSurchargeClient{}), http.MethodPost, disputePath, `{}`, nil).Code)
}

func TestListOpenDisputes_StaffGetsAnArrayAndOthersAre403(t *testing.T) {
	for _, role := range []string{"OPERATOR", "ADMIN"} {
		rec := do(surchargeRouter(&fakeSurchargeClient{}), http.MethodGet, disputesPath, "", authHdr(t, role))
		require.Equal(t, http.StatusOK, rec.Code, role)
		assert.JSONEq(t, `[{"id":"d-1","surchargeId":"s-1","customerId":"c-1","reason":"Yo estaba en casa","status":"OPEN","openedAt":"2026-10-30T12:00:00Z"}]`, rec.Body.String())
	}
	for _, role := range []string{"CUSTOMER", "DRIVER"} {
		fake := &fakeSurchargeClient{}
		rec := do(surchargeRouter(fake), http.MethodGet, disputesPath, "", authHdr(t, role))
		assert.Equal(t, http.StatusForbidden, rec.Code, role)
		assert.Equal(t, 0, fake.disputeCalls, role)
	}
}

func TestResolveDispute_StaffDecidesWithAReasonAndTheAnswerShowsTheAudit(t *testing.T) {
	for _, role := range []string{"OPERATOR", "ADMIN"} {
		fake := &fakeSurchargeClient{}

		rec := do(surchargeRouter(fake), http.MethodPost, resolutionPath, `{"decision":"IN_FAVOR","reason":"Revisamos la ruta del repartidor"}`, authHdr(t, role))

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, "d-1", fake.resolveDispReq.GetId())
		assert.Equal(t, "IN_FAVOR", fake.resolveDispReq.GetDecision())
		assert.Contains(t, rec.Body.String(), `"resolvedBy":"u-1"`)
		assert.Contains(t, rec.Body.String(), `"resolutionReason":"Revisamos la ruta del repartidor"`)
		assert.Contains(t, rec.Body.String(), `"resolvedAt":"2026-10-30T13:00:00Z"`)
	}
}

func TestResolveDispute_CustomerAndDriverAre403BeforeTheService(t *testing.T) {
	for _, role := range []string{"CUSTOMER", "DRIVER"} {
		fake := &fakeSurchargeClient{}
		rec := do(surchargeRouter(fake), http.MethodPost, resolutionPath, `{"decision":"IN_FAVOR","reason":"Revisamos la ruta"}`, authHdr(t, role))
		assert.Equal(t, http.StatusForbidden, rec.Code, role)
		assert.Equal(t, 0, fake.disputeCalls, role)
	}
}

func TestDispute_ServiceErrorsKeepTheirStatus(t *testing.T) {
	cases := map[codes.Code]int{
		codes.FailedPrecondition: http.StatusUnprocessableEntity,
		codes.Aborted:            http.StatusConflict,
		codes.NotFound:           http.StatusNotFound,
		codes.InvalidArgument:    http.StatusBadRequest,
	}
	for code, want := range cases {
		rec := do(surchargeRouter(&fakeSurchargeClient{err: status.Error(code, "x")}), http.MethodPost, disputePath, `{"reason":"Yo estaba en casa y nadie tocó"}`, authHdr(t, "CUSTOMER"))
		assert.Equal(t, want, rec.Code, code.String())
	}
}

func TestGetMyBalance_LinesCarryTheDisputeStateAndOmitItWhenThereIsNone(t *testing.T) {
	fake := &fakeSurchargeClient{balanceRes: &ordersv1.GetMyBalanceResponse{
		PendingSurcharge: "8.40", AccountStatus: "ACTIVO",
		Lines: []*ordersv1.BalanceLine{
			{Kind: "SURCHARGE", Id: "a", Reason: "CLIENT_REFUSED", Amount: "4.20", StartedAt: "2026-10-30T12:00:00Z", DueAt: "2026-11-13T12:00:00Z", DisputableUntil: "2026-10-30T14:00:00Z"},
			{Kind: "SURCHARGE", Id: "b", Reason: "CLIENT_ABSENT", Amount: "4.20", StartedAt: "2026-10-30T12:00:00Z", DueAt: "2026-11-13T12:00:00Z", DisputeStatus: "OPEN"},
			{Kind: "RECIDIVISM", Id: "c", Reason: "RECIDIVISM", Amount: "1.00", StartedAt: "2026-10-30T12:00:00Z", DueAt: "2026-11-13T12:00:00Z"},
		},
	}}

	rec := do(surchargeRouter(fake), http.MethodGet, balancePath, "", authHdr(t, "CUSTOMER"))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	body := rec.Body.String()
	assert.Contains(t, body, `"disputableUntil":"2026-10-30T14:00:00Z"`)
	assert.Contains(t, body, `"disputeStatus":"OPEN"`)
	assert.Equal(t, 1, strings.Count(body, `"disputeStatus"`), "only the disputed line carries a status")
	assert.Equal(t, 1, strings.Count(body, `"disputableUntil"`), "only the line that can still be disputed carries a deadline")
}
