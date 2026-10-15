package http_test

import (
	"context"
	"encoding/json"
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

type fakeOrderClient struct {
	createReq *ordersv1.CreateOrderRequest
	createRes *ordersv1.CreateOrderResponse
	reviewReq *ordersv1.ReviewOrderRequest
	cancelReq *ordersv1.CancelOrderRequest
	meta      grpcclient.CallMeta
	err       error
}

func (f *fakeOrderClient) CreateOrder(_ context.Context, m grpcclient.CallMeta, req *ordersv1.CreateOrderRequest) (*ordersv1.CreateOrderResponse, error) {
	f.createReq, f.meta = req, m
	if f.err != nil {
		return nil, f.err
	}
	if f.createRes != nil {
		return f.createRes, nil
	}
	return &ordersv1.CreateOrderResponse{Order: &ordersv1.Order{
		Id:            "o-1",
		Address:       req.GetAddress(),
		Status:        ordersv1.OrderStatus_ORDER_STATUS_PENDING_REVIEW,
		ReviewReasons: []ordersv1.ReviewReason{ordersv1.ReviewReason_REVIEW_REASON_ZONE_NOT_COVERED},
		CreatedAt:     "2030-04-10T15:00:00Z",
	}}, nil
}

func (f *fakeOrderClient) ListMyOrders(_ context.Context, m grpcclient.CallMeta, _ *ordersv1.ListMyOrdersRequest) (*ordersv1.ListMyOrdersResponse, error) {
	f.meta = m
	if f.err != nil {
		return nil, f.err
	}
	return &ordersv1.ListMyOrdersResponse{Orders: []*ordersv1.Order{{Id: "o-1", Status: ordersv1.OrderStatus_ORDER_STATUS_PENDING}}}, nil
}

func (f *fakeOrderClient) ListMyNotifications(_ context.Context, m grpcclient.CallMeta, _ *ordersv1.ListMyNotificationsRequest) (*ordersv1.ListMyNotificationsResponse, error) {
	f.meta = m
	if f.err != nil {
		return nil, f.err
	}
	return &ordersv1.ListMyNotificationsResponse{Notifications: []*ordersv1.Notification{{
		Id: "n-1", OrderId: "o-1", Kind: "ORDER_IN_REVIEW", Detail: "ZONE_NOT_COVERED", CreatedAt: "2030-04-10T15:00:00Z",
	}}}, nil
}

func (f *fakeOrderClient) ListOrderExceptions(_ context.Context, m grpcclient.CallMeta, _ *ordersv1.ListOrderExceptionsRequest) (*ordersv1.ListOrderExceptionsResponse, error) {
	f.meta = m
	if f.err != nil {
		return nil, f.err
	}
	return &ordersv1.ListOrderExceptionsResponse{Orders: []*ordersv1.Order{{Id: "o-9", Status: ordersv1.OrderStatus_ORDER_STATUS_PENDING_REVIEW}}}, nil
}

func (f *fakeOrderClient) ReviewOrder(_ context.Context, m grpcclient.CallMeta, req *ordersv1.ReviewOrderRequest) (*ordersv1.ReviewOrderResponse, error) {
	f.reviewReq, f.meta = req, m
	if f.err != nil {
		return nil, f.err
	}
	return &ordersv1.ReviewOrderResponse{Order: &ordersv1.Order{Id: req.GetId(), Status: ordersv1.OrderStatus_ORDER_STATUS_PENDING}}, nil
}

func ordersRouter(c httpapi.OrderClient) http.Handler {
	return newRouterWithSignup(&fakeAuth{}, httpapi.Options{Orders: c})
}

const validOrderBody = `{"address":"Avenida Amazonas 123","deliveryDate":"2030-04-12","windowStart":"09:00","windowEnd":"12:00"}`

func TestCreateOrder_AddressOnlyIsAccepted_CoordinatesStayAbsent(t *testing.T) {
	fake := &fakeOrderClient{}
	rec := do(ordersRouter(fake), http.MethodPost, "/api/v1/orders", validOrderBody, authHdr(t, "CUSTOMER"))

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var body struct {
		Status        string   `json:"status"`
		ReviewReasons []string `json:"reviewReasons"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "PENDING_REVIEW", body.Status)
	assert.Equal(t, []string{"ZONE_NOT_COVERED"}, body.ReviewReasons)
	assert.Equal(t, "CUSTOMER", fake.meta.Role, "la identidad del JWT se propaga al servicio")
	assert.Nil(t, fake.createReq.Latitude, "sin punto en el mapa el backend geocodifica")
	assert.Nil(t, fake.createReq.Longitude)
}

func TestCreateOrder_ReturnsProductsAndTotal(t *testing.T) {
	fake := &fakeOrderClient{createRes: &ordersv1.CreateOrderResponse{
		Order: &ordersv1.Order{Id: "o-2", Status: ordersv1.OrderStatus_ORDER_STATUS_PENDING},
		Items: []*ordersv1.OrderLine{{ProductId: "p-1", Name: "Manzana", UnitPrice: "1.50", Quantity: 2}},
		Total: "3.00",
	}}
	body := `{"address":"Avenida Amazonas 123","deliveryDate":"2030-04-12","windowStart":"09:00","windowEnd":"12:00",` +
		`"items":[{"productId":"11111111-1111-1111-1111-111111111111","quantity":2}]}`
	rec := do(ordersRouter(fake), http.MethodPost, "/api/v1/orders", body, authHdr(t, "CUSTOMER"))

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Len(t, fake.createReq.GetItems(), 1, "los productos llegan al servicio")
	assert.Equal(t, int64(2), fake.createReq.GetItems()[0].GetQuantity())
	var got struct {
		ID    string `json:"id"`
		Total string `json:"total"`
		Items []struct {
			ProductID string `json:"productId"`
			Name      string `json:"name"`
			UnitPrice string `json:"unitPrice"`
			Quantity  int64  `json:"quantity"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "o-2", got.ID, "los campos del pedido siguen en la raíz")
	assert.Equal(t, "3.00", got.Total)
	require.Len(t, got.Items, 1)
	assert.Equal(t, "p-1", got.Items[0].ProductID)
	assert.Equal(t, "Manzana", got.Items[0].Name)
	assert.Equal(t, "1.50", got.Items[0].UnitPrice)
	assert.Equal(t, int64(2), got.Items[0].Quantity)
}

func TestCreateOrder_WithMapPointKeepsCoordinates(t *testing.T) {
	fake := &fakeOrderClient{}
	body := `{"address":"Avenida Amazonas 123","latitude":0,"longitude":-78.48,"deliveryDate":"2030-04-12","windowStart":"09:00","windowEnd":"12:00"}`
	rec := do(ordersRouter(fake), http.MethodPost, "/api/v1/orders", body, authHdr(t, "CUSTOMER"))

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.NotNil(t, fake.createReq.Latitude, "una latitud 0 válida llega como presente")
	assert.InDelta(t, 0.0, *fake.createReq.Latitude, 1e-9)
}

func TestCreateOrder_WithoutToken_Returns401(t *testing.T) {
	rec := do(ordersRouter(&fakeOrderClient{}), http.MethodPost, "/api/v1/orders", validOrderBody, nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestCreateOrder_ZoneOrGeocoderDown_Returns503(t *testing.T) {
	fake := &fakeOrderClient{err: status.Error(codes.Unavailable, "zone down")}
	rec := do(ordersRouter(fake), http.MethodPost, "/api/v1/orders", validOrderBody, authHdr(t, "CUSTOMER"))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestListMyOrders_ReturnsEnvelope(t *testing.T) {
	rec := do(ordersRouter(&fakeOrderClient{}), http.MethodGet, "/api/v1/orders/mine", "", authHdr(t, "CUSTOMER"))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		Orders []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"orders"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Orders, 1)
	assert.Equal(t, "PENDING", body.Orders[0].Status)
}

func TestListMyOrders_WithoutToken_Returns401(t *testing.T) {
	rec := do(ordersRouter(&fakeOrderClient{}), http.MethodGet, "/api/v1/orders/mine", "", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestListExceptions_ReturnsReviewQueue(t *testing.T) {
	rec := do(ordersRouter(&fakeOrderClient{}), http.MethodGet, "/api/v1/orders/exceptions", "", authHdr(t, "OPERATOR"))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		Orders []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"orders"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Orders, 1)
	assert.Equal(t, "PENDING_REVIEW", body.Orders[0].Status)
}

func TestReview_CorrectSendsOnlyTheFieldsTheOperatorChanged(t *testing.T) {
	fake := &fakeOrderClient{}
	body := `{"action":"CORRECT","reason":"Dirección completada con el cliente","correction":{"address":"Calle Germán Alemán 555, Quito"}}`
	rec := do(ordersRouter(fake), http.MethodPost, "/api/v1/orders/o-7/review", body, authHdr(t, "OPERATOR"))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "o-7", fake.reviewReq.GetId())
	assert.Equal(t, ordersv1.ReviewAction_REVIEW_ACTION_CORRECT, fake.reviewReq.GetAction())
	assert.Equal(t, "OPERATOR", fake.meta.Role)
	require.NotNil(t, fake.reviewReq.GetCorrection().Address)
	assert.Nil(t, fake.reviewReq.GetCorrection().DeliveryDate, "un campo no enviado no se corrige")
}

func TestReview_UnknownActionIsBadRequest(t *testing.T) {
	body := `{"action":"DELETE","reason":"Motivo suficientemente largo"}`
	rec := do(ordersRouter(&fakeOrderClient{}), http.MethodPost, "/api/v1/orders/o-7/review", body, authHdr(t, "OPERATOR"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestReview_NotInReviewIsConflict(t *testing.T) {
	fake := &fakeOrderClient{err: status.Error(codes.Aborted, "order is not in review")}
	body := `{"action":"CONFIRM","reason":"Cliente verificado por teléfono"}`
	rec := do(ordersRouter(fake), http.MethodPost, "/api/v1/orders/o-7/review", body, authHdr(t, "OPERATOR"))
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestReview_SpatialConfirmIsUnprocessable(t *testing.T) {
	fake := &fakeOrderClient{err: status.Error(codes.FailedPrecondition, "must be corrected or rejected")}
	body := `{"action":"CONFIRM","reason":"Aceptar igual que está"}`
	rec := do(ordersRouter(fake), http.MethodPost, "/api/v1/orders/o-7/review", body, authHdr(t, "OPERATOR"))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestNotifications_ReturnsCustomerNotices(t *testing.T) {
	rec := do(ordersRouter(&fakeOrderClient{}), http.MethodGet, "/api/v1/notifications/mine", "", authHdr(t, "CUSTOMER"))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		Notifications []struct {
			Kind    string `json:"kind"`
			OrderID string `json:"orderId"`
		} `json:"notifications"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Notifications, 1)
	assert.Equal(t, "ORDER_IN_REVIEW", body.Notifications[0].Kind)
	assert.Equal(t, "o-1", body.Notifications[0].OrderID)
}

func (f *fakeOrderClient) CancelOrder(_ context.Context, m grpcclient.CallMeta, req *ordersv1.CancelOrderRequest) (*ordersv1.CancelOrderResponse, error) {
	f.cancelReq, f.meta = req, m
	if f.err != nil {
		return nil, f.err
	}
	return &ordersv1.CancelOrderResponse{Order: &ordersv1.Order{Id: req.GetId(), Status: ordersv1.OrderStatus_ORDER_STATUS_CANCELLED}}, nil
}

func TestCancelOrder_CustomerGets200WithCancelledOrder(t *testing.T) {
	fake := &fakeOrderClient{}
	rec := do(ordersRouter(fake), http.MethodPost, "/api/v1/orders/o-7/cancel", "", authHdr(t, "CUSTOMER"))

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "o-7", body.ID)
	assert.Equal(t, "CANCELLED", body.Status)
	require.NotNil(t, fake.cancelReq)
	assert.Equal(t, "o-7", fake.cancelReq.GetId())
	assert.Equal(t, "CUSTOMER", fake.meta.Role, "la identidad del JWT se propaga al servicio")
}

func TestCancelOrder_ServiceErrorsMapToHTTP(t *testing.T) {
	cases := map[string]struct {
		err  error
		want int
	}{
		"not cancellable": {status.Error(codes.Aborted, "order cannot be cancelled"), http.StatusConflict},
		"not owned":       {status.Error(codes.NotFound, "pedido no encontrado"), http.StatusNotFound},
		"not customer":    {status.Error(codes.PermissionDenied, "forbidden"), http.StatusForbidden},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fake := &fakeOrderClient{err: tc.err}
			rec := do(ordersRouter(fake), http.MethodPost, "/api/v1/orders/o-7/cancel", "", authHdr(t, "CUSTOMER"))
			assert.Equal(t, tc.want, rec.Code, rec.Body.String())
		})
	}
}
