package http_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	customerv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/customer/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
	httpapi "github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/http"
)

type fakeCustomerQueries struct {
	getReq    *customerv1.GetByIDRequest
	listReq   *customerv1.ListCustomersRequest
	statusReq *customerv1.UpdateStatusRequest
	err       error
	meta      grpcclient.CallMeta
}

func (f *fakeCustomerQueries) UpdateStatus(_ context.Context, m grpcclient.CallMeta, req *customerv1.UpdateStatusRequest) (*customerv1.UpdateStatusResponse, error) {
	f.statusReq, f.meta = req, m
	if f.err != nil {
		return nil, f.err
	}
	return &customerv1.UpdateStatusResponse{Customer: &customerv1.Customer{
		Id: req.GetId(), UserId: "u-1", FullName: "Ada", Status: req.GetStatus(),
	}}, nil
}

func (f *fakeCustomerQueries) GetByID(_ context.Context, m grpcclient.CallMeta, req *customerv1.GetByIDRequest) (*customerv1.GetByIDResponse, error) {
	f.getReq, f.meta = req, m
	if f.err != nil {
		return nil, f.err
	}
	return &customerv1.GetByIDResponse{Customer: &customerv1.Customer{
		Id: req.GetId(), UserId: "u-1", FullName: "Ada", Email: "ada@example.com",
		Status: "ACTIVO", PendingSurcharge: "0.00", CreatedAt: "2026-01-02T03:04:05Z", UpdatedAt: "2026-01-02T03:04:05Z",
	}}, nil
}

func (f *fakeCustomerQueries) ListCustomers(_ context.Context, m grpcclient.CallMeta, req *customerv1.ListCustomersRequest) (*customerv1.ListCustomersResponse, error) {
	f.listReq, f.meta = req, m
	if f.err != nil {
		return nil, f.err
	}
	return &customerv1.ListCustomersResponse{
		Customers: []*customerv1.Customer{{Id: "c-1", FullName: "Ada", Status: "ACTIVO"}},
		Total:     7,
	}, nil
}

func customersRouter(q httpapi.CustomerQueryClient) http.Handler {
	return newRouterWithSignup(&fakeAuth{}, httpapi.Options{CustomerQueries: q})
}

func authHdr(t *testing.T, role string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token(t, "u-1", role)}
}

func TestListCustomers_AsAdmin_Returns200WithPage(t *testing.T) {
	q := &fakeCustomerQueries{}
	rec := do(customersRouter(q), http.MethodGet, "/api/v1/customers?limit=10&offset=5&status=ACTIVO", "", authHdr(t, "ADMIN"))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, int32(10), q.listReq.GetLimit())
	assert.Equal(t, int32(5), q.listReq.GetOffset())
	assert.Equal(t, "ACTIVO", q.listReq.GetStatus())
	assert.Equal(t, "ADMIN", q.meta.Role)
	assert.Contains(t, rec.Body.String(), `"total":7`)
	assert.Contains(t, rec.Body.String(), `"fullName":"Ada"`)
	assert.Contains(t, rec.Body.String(), `"pendingSurcharge"`)
}

func TestListCustomers_NonAdminRoles_Return403(t *testing.T) {
	for _, role := range []string{"CUSTOMER", "OPERATOR", "DRIVER"} {
		q := &fakeCustomerQueries{}
		rec := do(customersRouter(q), http.MethodGet, "/api/v1/customers", "", authHdr(t, role))
		assert.Equal(t, http.StatusForbidden, rec.Code, role)
		assert.Nil(t, q.listReq, role)
	}
}

func TestListCustomers_WithoutAuth_Returns401(t *testing.T) {
	rec := do(customersRouter(&fakeCustomerQueries{}), http.MethodGet, "/api/v1/customers", "", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestListCustomers_NonNumericLimit_Returns400(t *testing.T) {
	q := &fakeCustomerQueries{}
	rec := do(customersRouter(q), http.MethodGet, "/api/v1/customers?limit=abc", "", authHdr(t, "ADMIN"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Nil(t, q.listReq)
}

func TestListCustomers_UpstreamInvalidArgument_Returns400(t *testing.T) {
	q := &fakeCustomerQueries{err: status.Error(codes.InvalidArgument, "bad")}
	rec := do(customersRouter(q), http.MethodGet, "/api/v1/customers?limit=-1", "", authHdr(t, "ADMIN"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGetCustomer_AsAdmin_Returns200(t *testing.T) {
	q := &fakeCustomerQueries{}
	rec := do(customersRouter(q), http.MethodGet, "/api/v1/customers/abc", "", authHdr(t, "ADMIN"))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "abc", q.getReq.GetId())
	assert.Contains(t, rec.Body.String(), `"userId":"u-1"`)
}

func TestGetCustomer_NotFound_Returns404(t *testing.T) {
	q := &fakeCustomerQueries{err: status.Error(codes.NotFound, "cliente no encontrado")}
	rec := do(customersRouter(q), http.MethodGet, "/api/v1/customers/abc", "", authHdr(t, "ADMIN"))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSetCustomerStatus_AsAdmin_Returns200AndForwardsStatus(t *testing.T) {
	q := &fakeCustomerQueries{}
	rec := do(customersRouter(q), http.MethodPatch, "/api/v1/customers/abc/status", `{"status":"BLOQUEADO_POR_SALDO"}`, authHdr(t, "ADMIN"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "abc", q.statusReq.GetId())
	assert.Equal(t, "BLOQUEADO_POR_SALDO", q.statusReq.GetStatus())
	assert.Contains(t, rec.Body.String(), `"status":"BLOQUEADO_POR_SALDO"`)
}

func TestSetCustomerStatus_AsOperator_Returns403(t *testing.T) {
	q := &fakeCustomerQueries{}
	rec := do(customersRouter(q), http.MethodPatch, "/api/v1/customers/abc/status", `{"status":"ACTIVO"}`, authHdr(t, "OPERATOR"))
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Nil(t, q.statusReq, "OPERATOR no llega al servicio")
}

func TestSetCustomerStatus_InvalidStatus_Returns400(t *testing.T) {
	q := &fakeCustomerQueries{err: status.Error(codes.InvalidArgument, "status inválido")}
	rec := do(customersRouter(q), http.MethodPatch, "/api/v1/customers/abc/status", `{"status":"NOPE"}`, authHdr(t, "ADMIN"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGetCustomer_AsCustomer_Returns403(t *testing.T) {
	rec := do(customersRouter(&fakeCustomerQueries{}), http.MethodGet, "/api/v1/customers/abc", "", authHdr(t, "CUSTOMER"))
	assert.Equal(t, http.StatusForbidden, rec.Code)
}
