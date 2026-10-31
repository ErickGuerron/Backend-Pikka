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
)

func (f *fakeSurchargeClient) SetReassignmentFee(_ context.Context, m grpcclient.CallMeta, req *ordersv1.SetReassignmentFeeRequest) (*ordersv1.SetReassignmentFeeResponse, error) {
	f.feeReq, f.meta = req, m
	if f.err != nil {
		return nil, f.err
	}
	return &ordersv1.SetReassignmentFeeResponse{Fee: &ordersv1.ReassignmentFee{
		Id: "f-1", Amount: "2.50", Reason: req.GetReason(), SetBy: "u-1", CreatedAt: "2026-10-30T09:00:00Z",
	}}, nil
}

func (f *fakeSurchargeClient) GetReassignmentFee(_ context.Context, m grpcclient.CallMeta, _ *ordersv1.GetReassignmentFeeRequest) (*ordersv1.GetReassignmentFeeResponse, error) {
	f.meta = m
	if f.err != nil {
		return nil, f.err
	}
	return &ordersv1.GetReassignmentFeeResponse{Fee: &ordersv1.ReassignmentFee{
		Id: "f-1", Amount: "2.50", Reason: "Sube la gasolina", SetBy: "u-1", CreatedAt: "2026-10-30T09:00:00Z",
	}}, nil
}

func (f *fakeSurchargeClient) ApplyReassignmentCharge(_ context.Context, m grpcclient.CallMeta, req *ordersv1.ApplyReassignmentChargeRequest) (*ordersv1.ApplyReassignmentChargeResponse, error) {
	f.reassignCalls++
	f.reassignReq, f.meta = req, m
	if f.err != nil {
		return nil, f.err
	}
	return &ordersv1.ApplyReassignmentChargeResponse{Charge: &ordersv1.ReassignmentCharge{
		Id: "r-1", CustomerId: "c-1", OrderId: req.GetOrderId(), Amount: "2.50", Reason: req.GetReason(), AppliedBy: "u-1", CreatedAt: "2026-10-30T09:00:00Z",
	}}, nil
}

const (
	feePath      = "/api/v1/settings/reassignment-fee"
	reassignPath = "/api/v1/orders/o-1/reassignment-charge"
)

func TestSetReassignmentFee_AdminForwardsAmountAndReason(t *testing.T) {
	fake := &fakeSurchargeClient{}

	rec := do(surchargeRouter(fake), http.MethodPut, feePath, `{"amount":"2.50","reason":"Sube la gasolina"}`, authHdr(t, "ADMIN"))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "2.50", fake.feeReq.GetAmount())
	assert.Equal(t, "Sube la gasolina", fake.feeReq.GetReason())
	assert.JSONEq(t, `{"id":"f-1","amount":"2.50","reason":"Sube la gasolina","setBy":"u-1","createdAt":"2026-10-30T09:00:00Z"}`, rec.Body.String())
}

func TestSetReassignmentFee_OnlyAdminReachesTheService(t *testing.T) {
	for _, role := range []string{"OPERATOR", "CUSTOMER", "DRIVER"} {
		fake := &fakeSurchargeClient{}
		rec := do(surchargeRouter(fake), http.MethodPut, feePath, `{"amount":"2.50","reason":"Sube la gasolina"}`, authHdr(t, role))
		assert.Equal(t, http.StatusForbidden, rec.Code, role)
		assert.Nil(t, fake.feeReq, role)
	}
	assert.Equal(t, http.StatusUnauthorized, do(surchargeRouter(&fakeSurchargeClient{}), http.MethodPut, feePath, `{}`, nil).Code)
}

func TestGetReassignmentFee_StaffSeesItAndOthersDoNot(t *testing.T) {
	for _, role := range []string{"OPERATOR", "ADMIN"} {
		rec := do(surchargeRouter(&fakeSurchargeClient{}), http.MethodGet, feePath, "", authHdr(t, role))
		require.Equal(t, http.StatusOK, rec.Code, role)
		assert.Contains(t, rec.Body.String(), `"amount":"2.50"`)
	}
	for _, role := range []string{"CUSTOMER", "DRIVER"} {
		assert.Equal(t, http.StatusForbidden, do(surchargeRouter(&fakeSurchargeClient{}), http.MethodGet, feePath, "", authHdr(t, role)).Code, role)
	}
}

func TestApplyReassignmentCharge_StaffSendsOnlyOrderAndReason(t *testing.T) {
	for _, role := range []string{"OPERATOR", "ADMIN"} {
		fake := &fakeSurchargeClient{}
		// An amount in the body must never matter: the charge is always the current fixed fee.
		body := `{"reason":"Pidió cambiar la entrega con el repartidor en camino","amount":"999"}`

		rec := do(surchargeRouter(fake), http.MethodPost, reassignPath, body, authHdr(t, role))

		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
		assert.Equal(t, "o-1", fake.reassignReq.GetOrderId())
		assert.Equal(t, "Pidió cambiar la entrega con el repartidor en camino", fake.reassignReq.GetReason())
		assert.JSONEq(t, `{"id":"r-1","customerId":"c-1","orderId":"o-1","amount":"2.50","reason":"Pidió cambiar la entrega con el repartidor en camino","appliedBy":"u-1","createdAt":"2026-10-30T09:00:00Z"}`, rec.Body.String())
	}
}

func TestApplyReassignmentCharge_CustomerAndDriverAre403BeforeTheService(t *testing.T) {
	for _, role := range []string{"CUSTOMER", "DRIVER"} {
		fake := &fakeSurchargeClient{}
		rec := do(surchargeRouter(fake), http.MethodPost, reassignPath, `{"reason":"Pidió cambiar la entrega"}`, authHdr(t, role))
		assert.Equal(t, http.StatusForbidden, rec.Code, role)
		assert.Equal(t, 0, fake.reassignCalls, role)
	}
}

func TestReassignment_ServiceErrorsKeepTheirStatus(t *testing.T) {
	cases := map[codes.Code]int{
		codes.FailedPrecondition: http.StatusUnprocessableEntity,
		codes.Aborted:            http.StatusConflict,
		codes.NotFound:           http.StatusNotFound,
		codes.InvalidArgument:    http.StatusBadRequest,
	}
	for code, want := range cases {
		rec := do(surchargeRouter(&fakeSurchargeClient{err: status.Error(code, "x")}), http.MethodPost, reassignPath, `{"reason":"Pidió cambiar la entrega"}`, authHdr(t, "OPERATOR"))
		assert.Equal(t, want, rec.Code, code.String())
	}
}
