package grpcclient

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	driversv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/drivers/v1"
	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	trackingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/tracking/v1"
)

// fakeOrdersService guarda la metadata saliente y devuelve un error gRPC fijo.
type fakeOrdersService struct {
	ordersv1.OrderServiceClient
	md  metadata.MD
	err error
}

func (f *fakeOrdersService) GetMyOrder(ctx context.Context, _ *ordersv1.GetMyOrderRequest, _ ...grpc.CallOption) (*ordersv1.GetMyOrderResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return nil, f.err
}

func (f *fakeOrdersService) StopsBeforeForOrder(ctx context.Context, _ *ordersv1.StopsBeforeForOrderRequest, _ ...grpc.CallOption) (*ordersv1.StopsBeforeForOrderResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return nil, f.err
}

func (f *fakeOrdersService) AdvanceRouteStop(ctx context.Context, _ *ordersv1.AdvanceRouteStopRequest, _ ...grpc.CallOption) (*ordersv1.AdvanceRouteStopResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return nil, f.err
}

// fakeDriversService cubre la búsqueda de repartidor por ruta.
type fakeDriversService struct {
	driversv1.DriverServiceClient
	md  metadata.MD
	err error
}

func (f *fakeDriversService) GetDriverByRouteID(ctx context.Context, _ *driversv1.GetDriverByRouteIDRequest, _ ...grpc.CallOption) (*driversv1.GetDriverByRouteIDResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return nil, f.err
}

// fakeTrackingService cubre la estimación de llegada.
type fakeTrackingService struct {
	trackingv1.TrackingServiceClient
	md  metadata.MD
	err error
}

func (f *fakeTrackingService) EstimateArrival(ctx context.Context, _ *trackingv1.EstimateArrivalRequest, _ ...grpc.CallOption) (*trackingv1.EstimateArrivalResponse, error) {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return nil, f.err
}

// wrapperCase llama a un wrapper y devuelve el error y la metadata que salió hacia el servicio.
type wrapperCase struct {
	name     string
	wantCode codes.Code
	call     func(ctx context.Context, m CallMeta) (metadata.MD, error)
}

// Los wrappers propagan la identidad del Gateway y devuelven el código gRPC del servicio sin cambiarlo.
func TestCustomerTrackingWrappers_ForwardIdentityAndKeepServiceCode(t *testing.T) {
	orders := &fakeOrdersService{err: status.Error(codes.NotFound, "pedido no encontrado")}
	ow := NewOrdersWithClient(orders, time.Second)
	drivers := &fakeDriversService{err: status.Error(codes.NotFound, "no driver")}
	dw := NewDriversWithClient(drivers, time.Second)
	tracking := &fakeTrackingService{err: status.Error(codes.FailedPrecondition, "stale")}
	tw := NewTrackingWithClient(tracking, time.Second)

	cases := []wrapperCase{
		{"GetMyOrder", codes.NotFound, func(ctx context.Context, m CallMeta) (metadata.MD, error) {
			_, err := ow.GetMyOrder(ctx, m, &ordersv1.GetMyOrderRequest{OrderId: "o-1"})
			return orders.md, err
		}},
		{"StopsBeforeForOrder", codes.NotFound, func(ctx context.Context, m CallMeta) (metadata.MD, error) {
			_, err := ow.StopsBeforeForOrder(ctx, m, &ordersv1.StopsBeforeForOrderRequest{OrderId: "o-1"})
			return orders.md, err
		}},
		{"AdvanceRouteStop", codes.NotFound, func(ctx context.Context, m CallMeta) (metadata.MD, error) {
			_, err := ow.AdvanceRouteStop(ctx, m, &ordersv1.AdvanceRouteStopRequest{RouteId: "zone-1#1", OrderId: "o-1"})
			return orders.md, err
		}},
		{"GetDriverByRouteID", codes.NotFound, func(ctx context.Context, m CallMeta) (metadata.MD, error) {
			_, err := dw.GetDriverByRouteID(ctx, m, &driversv1.GetDriverByRouteIDRequest{RouteId: "zone-1#1"})
			return drivers.md, err
		}},
		{"EstimateArrival", codes.FailedPrecondition, func(ctx context.Context, m CallMeta) (metadata.MD, error) {
			_, err := tw.EstimateArrival(ctx, m, &trackingv1.EstimateArrivalRequest{DriverId: "d-1"})
			return tracking.md, err
		}},
	}

	meta := CallMeta{RequestID: "req-1", UserID: "u-customer", Role: "CUSTOMER"}
	for _, tc := range cases {
		md, err := tc.call(context.Background(), meta)

		require.Error(t, err, tc.name)
		assert.Equal(t, tc.wantCode, status.Code(err), tc.name)
		assert.Equal(t, []string{"u-customer"}, md.Get(MetadataUserID), tc.name)
		assert.Equal(t, []string{"CUSTOMER"}, md.Get(MetadataUserRole), tc.name)
		assert.Equal(t, []string{"req-1"}, md.Get(MetadataRequestID), tc.name)
	}
}
