// Package grpc expone los casos de uso del Auth Service mediante gRPC.
package grpc

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	authv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/auth/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/auth-service/internal/domain"
)

// Claves de metadata con las que el API Gateway propaga la identidad y la correlación.
const (
	MetadataUserID    = "x-user-id"
	MetadataUserRole  = "x-user-role"
	MetadataRequestID = "x-request-id"
)

// UseCases agrupa los casos de uso que este adaptador necesita.
type UseCases struct {
	Login         *application.Login
	CreateUser    *application.CreateUser
	GetUser       *application.GetUser
	ValidateToken *application.ValidateToken
}

// Server implementa authv1.AuthServiceServer. Solo traduce; no contiene reglas de negocio.
type Server struct {
	authv1.UnimplementedAuthServiceServer
	uc UseCases
}

// NewServer crea el servidor gRPC.
func NewServer(uc UseCases) *Server {
	return &Server{uc: uc}
}

// Login autentica y emite un token.
func (s *Server) Login(ctx context.Context, req *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	res, err := s.uc.Login.Execute(ctx, req.GetEmail(), req.GetPassword())
	if err != nil {
		return nil, toStatus(err)
	}
	return &authv1.LoginResponse{
		AccessToken:      res.AccessToken,
		TokenType:        "Bearer",
		ExpiresInSeconds: int64(res.ExpiresIn / time.Second),
		User:             toProtoUser(res.User),
	}, nil
}

// CreateUser registra un usuario nuevo.
func (s *Server) CreateUser(ctx context.Context, req *authv1.CreateUserRequest) (*authv1.CreateUserResponse, error) {
	role, ok := fromProtoRole(req.GetRole())
	if !ok {
		return nil, status.Error(codes.InvalidArgument, domain.ErrInvalidRole.Error())
	}
	u, err := s.uc.CreateUser.Execute(ctx, callerFrom(ctx), domain.NewUserInput{
		Email:    req.GetEmail(),
		Password: req.GetPassword(),
		FullName: req.GetFullName(),
		Role:     role,
	})
	if err != nil {
		return nil, toStatus(err)
	}
	return &authv1.CreateUserResponse{User: toProtoUser(u)}, nil
}

// GetUser devuelve un usuario.
func (s *Server) GetUser(ctx context.Context, req *authv1.GetUserRequest) (*authv1.GetUserResponse, error) {
	u, err := s.uc.GetUser.Execute(ctx, callerFrom(ctx), req.GetId())
	if err != nil {
		return nil, toStatus(err)
	}
	return &authv1.GetUserResponse{User: toProtoUser(u)}, nil
}

// ValidateToken verifica un token.
func (s *Server) ValidateToken(_ context.Context, req *authv1.ValidateTokenRequest) (*authv1.ValidateTokenResponse, error) {
	c, err := s.uc.ValidateToken.Execute(req.GetAccessToken())
	if err != nil {
		return nil, toStatus(err)
	}
	return &authv1.ValidateTokenResponse{UserId: c.UserID, Role: toProtoRole(c.Role)}, nil
}

func callerFrom(ctx context.Context) ports.Caller {
	md, _ := metadata.FromIncomingContext(ctx)
	first := func(k string) string {
		if v := md.Get(k); len(v) > 0 {
			return v[0]
		}
		return ""
	}
	role, err := domain.ParseRole(first(MetadataUserRole))
	if err != nil {
		return ports.Caller{}
	}
	return ports.Caller{UserID: first(MetadataUserID), Role: role}
}

func toStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidEmail),
		errors.Is(err, domain.ErrInvalidPassword),
		errors.Is(err, domain.ErrInvalidFullName),
		errors.Is(err, domain.ErrInvalidRole):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrInvalidCredentials), errors.Is(err, domain.ErrInvalidToken):
		return status.Error(codes.Unauthenticated, "invalid credentials")
	case errors.Is(err, domain.ErrUserInactive):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, domain.ErrForbidden):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, domain.ErrUserNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrEmailAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "deadline exceeded")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "canceled")
	default:
		// El detalle queda en el log del interceptor; al cliente no se le filtra.
		return status.Error(codes.Internal, "internal error")
	}
}

func toProtoUser(u domain.User) *authv1.User {
	return &authv1.User{
		Id:        u.ID,
		Email:     u.Email,
		FullName:  u.FullName,
		Role:      toProtoRole(u.Role),
		Active:    u.Active,
		CreatedAt: u.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func toProtoRole(r domain.Role) authv1.Role {
	switch r {
	case domain.RoleAdmin:
		return authv1.Role_ROLE_ADMIN
	case domain.RoleOperator:
		return authv1.Role_ROLE_OPERATOR
	case domain.RoleDriver:
		return authv1.Role_ROLE_DRIVER
	default:
		return authv1.Role_ROLE_UNSPECIFIED
	}
}

func fromProtoRole(r authv1.Role) (domain.Role, bool) {
	switch r {
	case authv1.Role_ROLE_ADMIN:
		return domain.RoleAdmin, true
	case authv1.Role_ROLE_OPERATOR:
		return domain.RoleOperator, true
	case authv1.Role_ROLE_DRIVER:
		return domain.RoleDriver, true
	default:
		return "", false
	}
}
