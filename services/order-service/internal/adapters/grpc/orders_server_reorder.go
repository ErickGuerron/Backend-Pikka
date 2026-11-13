package grpc

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ordersv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/orders/v1"
	orderapp "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/order"
)

type RouteReorderer interface {
	ReorderDriverRoute(ctx context.Context, routeID, actorID, reason string, ordered []uuid.UUID, versions map[string]int64) (bool, error)
}

// The Gateway verifies ownership and that the proposed order respects delivery windows.
func (s *OrderServer) ReorderDriverRoute(ctx context.Context, req *ordersv1.ReorderDriverRouteRequest) (*ordersv1.ReorderDriverRouteResponse, error) {
	actor, role, err := identityFrom(ctx)
	if err != nil {
		return nil, err
	}
	if role != orderapp.RoleDriver {
		return nil, status.Error(codes.PermissionDenied, "only the driver can reorder a route")
	}
	if s.useCases.ReorderRoutes == nil {
		return nil, status.Error(codes.Unimplemented, "route reorder is not wired")
	}
	routeID, reason := strings.TrimSpace(req.GetRouteId()), strings.TrimSpace(req.GetReason())
	reasonLength := utf8.RuneCountInString(reason)
	if routeID == "" || reasonLength < 5 || reasonLength > 300 || strings.ContainsRune(reason, 0) || len(req.GetStops()) < 2 {
		return nil, status.Error(codes.InvalidArgument, "route, stops and a reason of 5 to 300 characters are required")
	}
	ordered := make([]uuid.UUID, 0, len(req.GetStops()))
	versions := make(map[string]int64, len(req.GetStops()))
	for _, stop := range req.GetStops() {
		id, err := uuid.Parse(stop.GetOrderId())
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid stop id")
		}
		if _, duplicate := versions[id.String()]; duplicate {
			return nil, status.Error(codes.InvalidArgument, "duplicate stop")
		}
		ordered = append(ordered, id)
		versions[id.String()] = stop.GetVersion()
	}
	ok, err := s.useCases.ReorderRoutes.ReorderDriverRoute(ctx, routeID, actor.String(), reason, ordered, versions)
	if err != nil {
		return nil, mapOrderError(err)
	}
	if !ok {
		return nil, status.Error(codes.Aborted, "route changed; reload and try again")
	}
	return &ordersv1.ReorderDriverRouteResponse{}, nil
}
