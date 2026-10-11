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

type fakeCustomers struct {
	errs  []error // un error por llamada; vacío => éxito
	calls []*customerv1.CreateCustomerRequest
	metas []grpcclient.CallMeta
}

func (f *fakeCustomers) CreateCustomer(_ context.Context, m grpcclient.CallMeta, req *customerv1.CreateCustomerRequest) (*customerv1.CreateCustomerResponse, error) {
	f.calls = append(f.calls, req)
	f.metas = append(f.metas, m)
	if i := len(f.calls) - 1; i < len(f.errs) && f.errs[i] != nil {
		return nil, f.errs[i]
	}
	return &customerv1.CreateCustomerResponse{}, nil
}

const signupBody = `{"email":"cliente@example.com","password":"password123","fullName":"Cliente Nuevo"}`

func signupWith(c httpapi.CustomerClient) http.Handler {
	return newRouterWithSignup(&signupFakeAuth{fakeAuth: &fakeAuth{}}, httpapi.Options{Customers: c})
}

func TestSignup_CreatesCustomerWithNewUserData(t *testing.T) {
	cc := &fakeCustomers{}
	rec := do(signupWith(cc), http.MethodPost, "/api/v1/auth/signup", signupBody, map[string]string{"X-Request-Id": "rid-1"})
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Len(t, cc.calls, 1)
	assert.Equal(t, "u-customer", cc.calls[0].GetUserId())
	assert.Equal(t, "cliente@example.com", cc.calls[0].GetEmail())
	assert.Equal(t, "Cliente Nuevo", cc.calls[0].GetFullName())
	assert.Equal(t, "rid-1", cc.metas[0].RequestID)
}

func TestSignup_CustomerAlreadyExists_IsSuccess(t *testing.T) {
	cc := &fakeCustomers{errs: []error{status.Error(codes.AlreadyExists, "dup")}}
	rec := do(signupWith(cc), http.MethodPost, "/api/v1/auth/signup", signupBody, nil)
	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Len(t, cc.calls, 1, "AlreadyExists no se reintenta")
}

func TestSignup_CustomerFailure_StillReturnsSignupResponse(t *testing.T) {
	cc := &fakeCustomers{errs: []error{status.Error(codes.Internal, "boom")}}
	rec := do(signupWith(cc), http.MethodPost, "/api/v1/auth/signup", signupBody, nil)
	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Contains(t, rec.Body.String(), "tok-customer")
	assert.Len(t, cc.calls, 1, "errores no transitorios no se reintentan")
}

func TestSignup_CustomerUnavailable_RetriesOnce(t *testing.T) {
	cc := &fakeCustomers{errs: []error{status.Error(codes.Unavailable, "down")}}
	rec := do(signupWith(cc), http.MethodPost, "/api/v1/auth/signup", signupBody, nil)
	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Len(t, cc.calls, 2)
}

func TestSignup_AuthFailure_DoesNotCreateCustomer(t *testing.T) {
	cc := &fakeCustomers{}
	body := `{"email":"duplicado@example.com","password":"password123","fullName":"X"}`
	rec := do(signupWith(cc), http.MethodPost, "/api/v1/auth/signup", body, nil)
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.Empty(t, cc.calls)
}
