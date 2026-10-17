package grpc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	routingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/routing/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/application/plan"
)

// fakeGenerate replaces the use case so the server tests only cover the gRPC mapping and the role gate.
type fakeGenerate struct {
	result plan.Result
	err    error
	calls  int
	date   string
}

func (f *fakeGenerate) Run(_ context.Context, deliveryDate string) (plan.Result, error) {
	f.calls++
	f.date = deliveryDate
	return f.result, f.err
}

func roleCtx(role string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(metadataUserRole, role))
}

func TestGeneratePlanRoleGate(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want codes.Code
	}{
		{"missing identity", context.Background(), codes.Unauthenticated},
		{"blank role", roleCtx(""), codes.Unauthenticated},
		{"customer role", roleCtx("CUSTOMER"), codes.PermissionDenied},
		{"driver role", roleCtx("DRIVER"), codes.PermissionDenied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen := &fakeGenerate{}
			_, err := NewRoutingServer(gen).GeneratePlan(tt.ctx, &routingv1.GeneratePlanRequest{DeliveryDate: "2026-10-15"})
			if status.Code(err) != tt.want {
				t.Fatalf("code = %v, want %v (err %v)", status.Code(err), tt.want, err)
			}
			if gen.calls != 0 {
				t.Fatalf("use case reached before the role gate rejected the call")
			}
		})
	}
}

func TestGeneratePlanAllowsAdminAndOperator(t *testing.T) {
	for _, role := range []string{"ADMIN", "OPERATOR"} {
		t.Run(role, func(t *testing.T) {
			gen := &fakeGenerate{}
			if _, err := NewRoutingServer(gen).GeneratePlan(roleCtx(role), &routingv1.GeneratePlanRequest{DeliveryDate: "2026-10-15"}); err != nil {
				t.Fatalf("GeneratePlan: %v", err)
			}
			if gen.calls != 1 || gen.date != "2026-10-15" {
				t.Fatalf("calls = %d date = %q", gen.calls, gen.date)
			}
		})
	}
}

func TestGeneratePlanMapsErrorsToStatusCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"invalid date", fmt.Errorf("generate: %w", plan.ErrInvalidDate), codes.InvalidArgument},
		{"no driver available", fmt.Errorf("route 2026-10-15/A#2: %w", plan.ErrNoDriverAvailable), codes.FailedPrecondition},
		{"any other failure", errors.New("order service unavailable"), codes.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen := &fakeGenerate{
				err:    tt.err,
				result: plan.Result{Routes: []plan.AssignedRoute{{RouteID: "2026-10-15/A#1"}}},
			}
			_, err := NewRoutingServer(gen).GeneratePlan(roleCtx("OPERATOR"), &routingv1.GeneratePlanRequest{DeliveryDate: "2026-10-15"})
			if status.Code(err) != tt.want {
				t.Fatalf("code = %v, want %v (err %v)", status.Code(err), tt.want, err)
			}
			if msg := status.Convert(err).Message(); !strings.Contains(msg, "routes assigned before the failure: 1") {
				t.Fatalf("message %q does not report the routes already assigned", msg)
			}
		})
	}
}

func TestGeneratePlanReturnsRoutesOnSuccess(t *testing.T) {
	gen := &fakeGenerate{result: plan.Result{Routes: []plan.AssignedRoute{
		{RouteID: "2026-10-15/zone-a#1", DriverID: "d1", OrderCount: 2},
		{RouteID: "2026-10-15/zone-a#2", DriverID: "d2", OrderCount: 1},
	}}}

	got, err := NewRoutingServer(gen).GeneratePlan(roleCtx("ADMIN"), &routingv1.GeneratePlanRequest{DeliveryDate: "2026-10-15"})
	if err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}
	routes := got.GetRoutes()
	if len(routes) != 2 {
		t.Fatalf("routes = %d, want 2", len(routes))
	}
	first := routes[0]
	if first.GetRouteId() != "2026-10-15/zone-a#1" || first.GetDriverId() != "d1" || first.GetOrderCount() != 2 {
		t.Fatalf("first route = %+v", first)
	}
	if routes[1].GetDriverId() != "d2" || routes[1].GetOrderCount() != 1 {
		t.Fatalf("second route = %+v", routes[1])
	}
}
