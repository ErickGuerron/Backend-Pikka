package grpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Metadata que el API Gateway propaga tras validar el JWT (ADR 0002).
const metadataUserRole = "x-user-role"

// Roles que pueden usar las operaciones de zonas (ADR 0008).
const (
	roleAdmin    = "ADMIN"
	roleOperator = "OPERATOR"
)

// requireRole decide si el llamante tiene uno de los roles permitidos.
// Sin identidad responde Unauthenticated; con identidad de otro rol, PermissionDenied.
// La decisión vive en este servicio: el Gateway no aplica reglas por rol (ADR 0002).
func requireRole(ctx context.Context, allowed ...string) error {
	md, _ := metadata.FromIncomingContext(ctx)
	roles := md.Get(metadataUserRole)
	if len(roles) == 0 || roles[0] == "" {
		return status.Error(codes.Unauthenticated, "missing identity")
	}
	for _, role := range allowed {
		if roles[0] == role {
			return nil
		}
	}
	return status.Error(codes.PermissionDenied, "forbidden")
}
