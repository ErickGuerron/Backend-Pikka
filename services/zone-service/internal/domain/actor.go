package domain

import "context"

// actorKey es la clave del contexto con el usuario que hace el cambio. Quien llama al dominio
// (el adaptador gRPC) lo fija a partir de la identidad que propagó el Gateway.
type actorKey struct{}

// WithActor devuelve un contexto con el identificador del usuario que realiza la operación.
func WithActor(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, actorKey{}, userID)
}

// ActorFrom devuelve el usuario que realiza la operación. Vacío si no se fijó.
func ActorFrom(ctx context.Context) string {
	id, _ := ctx.Value(actorKey{}).(string)
	return id
}
