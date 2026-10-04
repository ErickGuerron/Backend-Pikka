// Package grpcclient crea clientes gRPC hacia los servicios internos.
package grpcclient

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"

	authv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/auth/v1"
)

// Claves de metadata que los servicios internos leen para identidad y correlación.
const (
	MetadataUserID    = "x-user-id"
	MetadataUserRole  = "x-user-role"
	MetadataRequestID = "x-request-id"
)

// CallMeta es lo que el Gateway propaga en cada llamada interna.
type CallMeta struct {
	RequestID string
	UserID    string
	Role      string
}

// Dial abre una conexión gRPC perezosa hacia addr. La red interna es privada;
// mTLS queda como mejora posterior.
func Dial(addr string) (*grpc.ClientConn, error) {
	return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

// Auth envuelve al cliente del Auth Service imponiendo deadline y propagando metadata.
type Auth struct {
	client  authv1.AuthServiceClient
	health  healthpb.HealthClient
	timeout time.Duration
}

// NewAuth crea el envoltorio.
func NewAuth(conn grpc.ClientConnInterface, timeout time.Duration) *Auth {
	return &Auth{client: authv1.NewAuthServiceClient(conn), health: healthpb.NewHealthClient(conn), timeout: timeout}
}

// NewAuthWithClient permite inyectar un cliente falso en pruebas.
func NewAuthWithClient(client authv1.AuthServiceClient, timeout time.Duration) *Auth {
	return &Auth{client: client, timeout: timeout}
}

func (a *Auth) outgoing(ctx context.Context, m CallMeta) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	pairs := []string{MetadataRequestID, m.RequestID}
	if m.UserID != "" {
		pairs = append(pairs, MetadataUserID, m.UserID, MetadataUserRole, m.Role)
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...), cancel
}

// Login reenvía credenciales al Auth Service.
func (a *Auth) Login(ctx context.Context, m CallMeta, req *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	ctx, cancel := a.outgoing(ctx, m)
	defer cancel()
	return a.client.Login(ctx, req)
}

// CreateUser solicita el alta de un usuario.
func (a *Auth) CreateUser(ctx context.Context, m CallMeta, req *authv1.CreateUserRequest) (*authv1.CreateUserResponse, error) {
	ctx, cancel := a.outgoing(ctx, m)
	defer cancel()
	return a.client.CreateUser(ctx, req)
}

// GetUser consulta un usuario.
func (a *Auth) GetUser(ctx context.Context, m CallMeta, req *authv1.GetUserRequest) (*authv1.GetUserResponse, error) {
	ctx, cancel := a.outgoing(ctx, m)
	defer cancel()
	return a.client.GetUser(ctx, req)
}

// Ready verifica con el protocolo estándar de health de gRPC que el servicio atiende.
func (a *Auth) Ready(ctx context.Context) bool {
	if a.health == nil {
		return true
	}
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	res, err := a.health.Check(ctx, &healthpb.HealthCheckRequest{Service: authv1.AuthService_ServiceDesc.ServiceName})
	return err == nil && res.GetStatus() == healthpb.HealthCheckResponse_SERVING
}
