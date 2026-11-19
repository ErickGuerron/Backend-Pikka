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

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
)

// penaltyOrdersService records the outgoing metadata of the penalty calls and answers with a fixed gRPC error.
type penaltyOrdersService struct {
	ordersv1.OrderServiceClient
	md  metadata.MD
	err error
}

func (f *penaltyOrdersService) seen(ctx context.Context) error {
	f.md, _ = metadata.FromOutgoingContext(ctx)
	return f.err
}

func (f *penaltyOrdersService) ListRecidivists(ctx context.Context, _ *ordersv1.ListRecidivistsRequest, _ ...grpc.CallOption) (*ordersv1.ListRecidivistsResponse, error) {
	return nil, f.seen(ctx)
}

func (f *penaltyOrdersService) ClearRecidivist(ctx context.Context, _ *ordersv1.ClearRecidivistRequest, _ ...grpc.CallOption) (*ordersv1.ClearRecidivistResponse, error) {
	return nil, f.seen(ctx)
}

func (f *penaltyOrdersService) SetReassignmentFee(ctx context.Context, _ *ordersv1.SetReassignmentFeeRequest, _ ...grpc.CallOption) (*ordersv1.SetReassignmentFeeResponse, error) {
	return nil, f.seen(ctx)
}

func (f *penaltyOrdersService) GetReassignmentFee(ctx context.Context, _ *ordersv1.GetReassignmentFeeRequest, _ ...grpc.CallOption) (*ordersv1.GetReassignmentFeeResponse, error) {
	return nil, f.seen(ctx)
}

func (f *penaltyOrdersService) ApplyReassignmentCharge(ctx context.Context, _ *ordersv1.ApplyReassignmentChargeRequest, _ ...grpc.CallOption) (*ordersv1.ApplyReassignmentChargeResponse, error) {
	return nil, f.seen(ctx)
}

func TestOrdersPenaltyCallsForwardIdentityAndKeepTheServiceError(t *testing.T) {
	meta := CallMeta{RequestID: "req-1", UserID: "u-1", Role: "ADMIN"}
	calls := map[string]func(o *Orders) error{
		"ListRecidivists": func(o *Orders) error {
			_, err := o.ListRecidivists(context.Background(), meta, &ordersv1.ListRecidivistsRequest{})
			return err
		},
		"ClearRecidivist": func(o *Orders) error {
			_, err := o.ClearRecidivist(context.Background(), meta, &ordersv1.ClearRecidivistRequest{})
			return err
		},
		"SetReassignmentFee": func(o *Orders) error {
			_, err := o.SetReassignmentFee(context.Background(), meta, &ordersv1.SetReassignmentFeeRequest{})
			return err
		},
		"GetReassignmentFee": func(o *Orders) error {
			_, err := o.GetReassignmentFee(context.Background(), meta, &ordersv1.GetReassignmentFeeRequest{})
			return err
		},
		"ApplyReassignmentCharge": func(o *Orders) error {
			_, err := o.ApplyReassignmentCharge(context.Background(), meta, &ordersv1.ApplyReassignmentChargeRequest{})
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			fake := &penaltyOrdersService{err: status.Error(codes.PermissionDenied, "no")}

			err := call(NewOrdersWithClient(fake, time.Second))

			require.Error(t, err)
			assert.Equal(t, codes.PermissionDenied, status.Code(err))
			assert.Equal(t, []string{"u-1"}, fake.md.Get(MetadataUserID))
			assert.Equal(t, []string{"ADMIN"}, fake.md.Get(MetadataUserRole))
			assert.Equal(t, []string{"req-1"}, fake.md.Get(MetadataRequestID))
		})
	}
}
