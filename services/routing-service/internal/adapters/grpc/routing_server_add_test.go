package grpc

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	routingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/routing/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/application/plan"
	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

type fakeAdder struct {
	stop    domain.RouteStop
	err     error
	calls   int
	gotArgs [2]string
}

func (f *fakeAdder) Run(_ context.Context, routeID, orderID string) (domain.RouteStop, error) {
	f.calls++
	f.gotArgs = [2]string{routeID, orderID}
	return f.stop, f.err
}

func TestGeneratePlanDryRunOnlyPreviews(t *testing.T) {
	gen := &fakeGenerate{}
	if _, err := NewRoutingServer(gen).GeneratePlan(roleCtx("OPERATOR"), &routingv1.GeneratePlanRequest{DeliveryDate: "2026-10-15", DryRun: true}); err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}
	if gen.previews != 1 || gen.calls != 0 {
		t.Fatalf("previews = %d, runs = %d; a dry run must never run the real plan", gen.previews, gen.calls)
	}
}

func TestAddOrderToRouteRoleGateAndWiring(t *testing.T) {
	adder := &fakeAdder{}
	srv := NewRoutingServer(&fakeGenerate{}).WithRouteAdder(adder)
	req := &routingv1.AddOrderToRouteRequest{RouteId: "r1", OrderId: "o1"}

	if _, err := srv.AddOrderToRoute(context.Background(), req); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("no identity: %v", err)
	}
	if _, err := srv.AddOrderToRoute(roleCtx("CUSTOMER"), req); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("customer: %v", err)
	}
	if adder.calls != 0 {
		t.Fatal("use case reached before the role gate")
	}
	if _, err := NewRoutingServer(&fakeGenerate{}).AddOrderToRoute(roleCtx("ADMIN"), req); status.Code(err) != codes.Unimplemented {
		t.Fatalf("not wired: %v", err)
	}
}

func TestAddOrderToRouteReturnsTheNewStop(t *testing.T) {
	adder := &fakeAdder{stop: domain.RouteStop{OrderID: "o1", Address: "Calle 1", Sequence: 4}}
	res, err := NewRoutingServer(&fakeGenerate{}).WithRouteAdder(adder).
		AddOrderToRoute(roleCtx("OPERATOR"), &routingv1.AddOrderToRouteRequest{RouteId: "r1", OrderId: "o1"})
	if err != nil {
		t.Fatalf("AddOrderToRoute: %v", err)
	}
	if adder.gotArgs != [2]string{"r1", "o1"} || res.GetRouteId() != "r1" || res.GetStop().GetSequence() != 4 || res.GetStop().GetAddress() != "Calle 1" {
		t.Fatalf("args = %v, response = %v", adder.gotArgs, res)
	}
}

func TestAddOrderToRouteMapsErrorsToGRPCCodes(t *testing.T) {
	tests := []struct {
		err  error
		want codes.Code
	}{
		{plan.ErrBlankID, codes.InvalidArgument},
		{plan.ErrRouteNotFound, codes.NotFound},
		{plan.ErrOrderNotPending, codes.FailedPrecondition},
		{plan.ErrZoneMismatch, codes.FailedPrecondition},
		{plan.ErrRouteFinished, codes.FailedPrecondition},
		{plan.ErrRouteFull, codes.FailedPrecondition},
		{errors.New("order service down"), codes.Internal},
	}
	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			srv := NewRoutingServer(&fakeGenerate{}).WithRouteAdder(&fakeAdder{err: tt.err})
			_, err := srv.AddOrderToRoute(roleCtx("OPERATOR"), &routingv1.AddOrderToRouteRequest{RouteId: "r", OrderId: "o"})
			if status.Code(err) != tt.want {
				t.Fatalf("code = %v, want %v", status.Code(err), tt.want)
			}
		})
	}
}
