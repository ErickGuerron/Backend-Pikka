package grpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/domain"
)

// Metadata que el API Gateway propaga tras validar el JWT (ADR 0002).
const (
	metadataUserRole = "x-user-role"
	metadataUserID   = "x-user-id"
)

// Roles que pueden usar las operaciones de zonas (ADR 0008).
const (
	roleAdmin    = "ADMIN"
	roleOperator = "OPERATOR"
	// roleService identifica llamadas internas entre servicios (Order Service
	// clasifica pedidos). El Gateway nunca lo emite: su rol sale del JWT y auth
	// no emite este valor, así que un cliente externo no puede obtenerlo.
	roleService = "SERVICE"
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

// withActor fija en el contexto quién hace el cambio, para la bitácora. Sin usuario identificado
// no se puede auditar, así que la operación se rechaza.
func withActor(ctx context.Context) (context.Context, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	ids := md.Get(metadataUserID)
	if len(ids) == 0 || ids[0] == "" {
		return ctx, status.Error(codes.Unauthenticated, "missing user identity")
	}
	return domain.WithActor(ctx, ids[0]), nil
}
