// Package grpc exposes the routing.v1 RoutingService over gRPC.
package grpc

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	routingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/routing/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/application/plan"
	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

// metadataUserRole carries the caller's role, propagated by the Gateway (ADR 0002).
const metadataUserRole = "x-user-role"

// Roles allowed to generate a plan (ADR 0008).
const (
	roleAdmin    = "ADMIN"
	roleOperator = "OPERATOR"
)

// planRunner is the use case the server calls. *plan.Generate implements it.
type planRunner interface {
	Run(ctx context.Context, deliveryDate string) (plan.Result, error)
}

// RoutingServer implements routingv1.RoutingServiceServer.
type RoutingServer struct {
	routingv1.UnimplementedRoutingServiceServer
	generate planRunner
}

var _ routingv1.RoutingServiceServer = (*RoutingServer)(nil)

// NewRoutingServer wraps the plan use case.
func NewRoutingServer(generate planRunner) *RoutingServer {
	return &RoutingServer{generate: generate}
}

// GeneratePlan checks the role before any downstream call, then runs the use case and maps its result.
func (s *RoutingServer) GeneratePlan(ctx context.Context, req *routingv1.GeneratePlanRequest) (*routingv1.GeneratePlanResponse, error) {
	if err := requirePlannerRole(ctx); err != nil {
		return nil, err
	}
	result, err := s.generate.Run(ctx, req.GetDeliveryDate())
	if err != nil {
		return nil, mapGenerateError(err, len(result.Routes))
	}
	// Result.Routes and Result.Assigned are appended together, so index i describes the same route.
	// The guard keeps the server safe if a result ever carries routes without their planned stops.
	routes := make([]*routingv1.PlannedRoute, 0, len(result.Routes))
	for i, route := range result.Routes {
		var stops []*routingv1.PlannedStop
		if i < len(result.Assigned) {
			stops = plannedStops(result.Assigned[i].Stops)
		}
		routes = append(routes, &routingv1.PlannedRoute{
			RouteId:    route.RouteID,
			DriverId:   route.DriverID,
			OrderCount: int32(route.OrderCount),
			DriverName: route.DriverName,
			Stops:      stops,
		})
	}
	return &routingv1.GeneratePlanResponse{
		Routes:             routes,
		PlanId:             planID(req.GetDeliveryDate()),
		UnassignedOrderIds: []string{},
	}, nil
}

// planID derives the plan id of a delivery date. Phase 1: it is not persisted, so it is only a label.
func planID(deliveryDate string) string {
	return "plan-" + deliveryDate
}

// plannedStops maps the planner's stops to the contract, keeping their sequence.
func plannedStops(stops []domain.RouteStop) []*routingv1.PlannedStop {
	out := make([]*routingv1.PlannedStop, 0, len(stops))
	for _, stop := range stops {
		out = append(out, &routingv1.PlannedStop{
			OrderId:  stop.OrderID,
			Address:  stop.Address,
			Sequence: int32(stop.Sequence),
		})
	}
	return out
}

// requirePlannerRole rejects a missing or blank role as UNAUTHENTICATED and any role other than
// ADMIN or OPERATOR as PERMISSION_DENIED.
func requirePlannerRole(ctx context.Context) error {
	md, _ := metadata.FromIncomingContext(ctx)
	roles := md.Get(metadataUserRole)
	if len(roles) == 0 || strings.TrimSpace(roles[0]) == "" {
		return status.Error(codes.Unauthenticated, "missing identity")
	}
	switch roles[0] {
	case roleAdmin, roleOperator:
		return nil
	default:
		return status.Error(codes.PermissionDenied, "role cannot generate routing plans")
	}
}

// mapGenerateError turns a use case error into a gRPC status. The message reports how many routes
// were assigned before the failure, because those routes are not rolled back.
func mapGenerateError(err error, assigned int) error {
	var code codes.Code
	switch {
	case errors.Is(err, plan.ErrInvalidDate):
		code = codes.InvalidArgument
	case errors.Is(err, plan.ErrNoDriverAvailable):
		code = codes.FailedPrecondition
	default:
		code = codes.Internal
	}
	return status.Errorf(code, "%v (routes assigned before the failure: %d)", err, assigned)
}
