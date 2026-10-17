package grpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// metadataUserRole carries the caller's role, forwarded by the Gateway.
const (
	metadataUserRole = "x-user-role"
	roleAdmin        = "ADMIN"
)

// requireRole allows the call only when the caller's role is exactly role. A missing role
// or any other role is PERMISSION_DENIED.
func requireRole(ctx context.Context, role string) error {
	md, _ := metadata.FromIncomingContext(ctx)
	roles := md.Get(metadataUserRole)
	if len(roles) == 1 && roles[0] == role {
		return nil
	}
	return status.Error(codes.PermissionDenied, "forbidden")
}
