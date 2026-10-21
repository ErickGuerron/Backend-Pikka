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
	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
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

// Phase 1 contract: driver name, ordered stops with addresses, a derived plan id, and no unassigned orders.
func TestGeneratePlanMapsNamesStopsAndPlanID(t *testing.T) {
	gen := &fakeGenerate{result: plan.Result{
		Routes: []plan.AssignedRoute{{RouteID: "2026-10-15-zone-a-1", DriverID: "d1", DriverName: "Luis Andrade", OrderCount: 2}},
		Assigned: []domain.Route{{ID: "zone-a#1", ZoneID: "zone-a", Stops: []domain.RouteStop{
			{OrderID: "o-1", ZoneID: "zone-a", Address: "Av. Amazonas 100", Sequence: 1},
			{OrderID: "o-2", ZoneID: "zone-a", Address: "Calle 9 de Octubre 20", Sequence: 2},
		}}},
	}}

	got, err := NewRoutingServer(gen).GeneratePlan(roleCtx("OPERATOR"), &routingv1.GeneratePlanRequest{DeliveryDate: "2026-10-15"})
	if err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}
	if got.GetPlanId() != "plan-2026-10-15" {
		t.Fatalf("plan id = %q, want plan-2026-10-15", got.GetPlanId())
	}
	if len(got.GetUnassignedOrderIds()) != 0 {
		t.Fatalf("unassigned = %v, want none in phase 1", got.GetUnassignedOrderIds())
	}
	route := got.GetRoutes()[0]
	if route.GetDriverName() != "Luis Andrade" {
		t.Fatalf("driver name = %q", route.GetDriverName())
	}
	stops := route.GetStops()
	if len(stops) != 2 {
		t.Fatalf("stops = %d, want 2", len(stops))
	}
	if stops[0].GetOrderId() != "o-1" || stops[0].GetAddress() != "Av. Amazonas 100" || stops[0].GetSequence() != 1 {
		t.Fatalf("first stop = %+v", stops[0])
	}
	if stops[1].GetOrderId() != "o-2" || stops[1].GetAddress() != "Calle 9 de Octubre 20" || stops[1].GetSequence() != 2 {
		t.Fatalf("second stop = %+v", stops[1])
	}
}
