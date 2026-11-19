package grpcclient

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	routingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/routing/v1"
)

// Routing wraps the Routing Service client used to generate the route plan.
// It enforces a deadline and forwards the Gateway identity, like Tracking does.
type Routing struct {
	client  routingv1.RoutingServiceClient
	timeout time.Duration
}

// NewRouting wraps an existing gRPC connection.
func NewRouting(conn grpc.ClientConnInterface, timeout time.Duration) *Routing {
	return &Routing{client: routingv1.NewRoutingServiceClient(conn), timeout: timeout}
}

// NewRoutingWithClient injects a fake client in tests.
func NewRoutingWithClient(client routingv1.RoutingServiceClient, timeout time.Duration) *Routing {
	return &Routing{client: client, timeout: timeout}
}

func (r *Routing) outgoing(ctx context.Context, m CallMeta) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	pairs := []string{MetadataRequestID, m.RequestID}
	if m.UserID != "" {
		pairs = append(pairs, MetadataUserID, m.UserID, MetadataUserRole, m.Role)
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...), cancel
}

// GeneratePlan asks the Routing Service for the route plan of one date. Errors are returned as is:
// the handler maps them with the same table as the rest of the API.
func (r *Routing) GeneratePlan(ctx context.Context, m CallMeta, req *routingv1.GeneratePlanRequest) (*routingv1.GeneratePlanResponse, error) {
	ctx, cancel := r.outgoing(ctx, m)
	defer cancel()
	return r.client.GeneratePlan(ctx, req)
}
