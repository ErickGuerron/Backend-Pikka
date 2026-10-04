package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	authv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/auth/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
)

// AuthClient es lo que los handlers necesitan del Auth Service.
type AuthClient interface {
	Login(ctx context.Context, m grpcclient.CallMeta, req *authv1.LoginRequest) (*authv1.LoginResponse, error)
	CreateUser(ctx context.Context, m grpcclient.CallMeta, req *authv1.CreateUserRequest) (*authv1.CreateUserResponse, error)
	GetUser(ctx context.Context, m grpcclient.CallMeta, req *authv1.GetUserRequest) (*authv1.GetUserResponse, error)
}

// Los handlers solo traducen REST <-> gRPC. La autorización y las reglas viven en los servicios.
type authHandlers struct {
	auth AuthClient
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	AccessToken string  `json:"accessToken"`
	TokenType   string  `json:"tokenType"`
	ExpiresIn   int64   `json:"expiresIn"`
	User        userDTO `json:"user"`
}

type createUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"fullName"`
	Role     string `json:"role"`
}

type userDTO struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	FullName  string `json:"fullName"`
	Role      string `json:"role"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"createdAt"`
}

func (h *authHandlers) login(c echo.Context) error {
	var req loginRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	res, err := h.auth.Login(c.Request().Context(), callMeta(c), &authv1.LoginRequest{Email: req.Email, Password: req.Password})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, loginResponse{
		AccessToken: res.GetAccessToken(),
		TokenType:   res.GetTokenType(),
		ExpiresIn:   res.GetExpiresInSeconds(),
		User:        toUserDTO(res.GetUser()),
	})
}

func (h *authHandlers) me(c echo.Context) error {
	id, _ := identityFrom(c)
	return h.respondUser(c, id.UserID)
}

func (h *authHandlers) getUser(c echo.Context) error {
	return h.respondUser(c, c.Param("id"))
}

func (h *authHandlers) respondUser(c echo.Context, userID string) error {
	res, err := h.auth.GetUser(c.Request().Context(), callMeta(c), &authv1.GetUserRequest{Id: userID})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, toUserDTO(res.GetUser()))
}

func (h *authHandlers) createUser(c echo.Context) error {
	var req createUserRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	res, err := h.auth.CreateUser(c.Request().Context(), callMeta(c), &authv1.CreateUserRequest{
		Email:    req.Email,
		Password: req.Password,
		FullName: req.FullName,
		Role:     roleToProto(req.Role),
	})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusCreated, toUserDTO(res.GetUser()))
}

func bindJSON(c echo.Context, dst any) error {
	if ct := c.Request().Header.Get(echo.HeaderContentType); !isJSON(ct) {
		return echo.NewHTTPError(http.StatusUnsupportedMediaType, "content type must be application/json")
	}
	if err := (&echo.DefaultBinder{}).BindBody(c, dst); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "malformed JSON body")
	}
	return nil
}

func isJSON(ct string) bool {
	return strings.HasPrefix(strings.ToLower(ct), echo.MIMEApplicationJSON)
}

func toUserDTO(u *authv1.User) userDTO {
	return userDTO{
		ID:        u.GetId(),
		Email:     u.GetEmail(),
		FullName:  u.GetFullName(),
		Role:      roleFromProto(u.GetRole()),
		Active:    u.GetActive(),
		CreatedAt: u.GetCreatedAt(),
	}
}

func roleToProto(r string) authv1.Role {
	switch r {
	case "ADMIN":
		return authv1.Role_ROLE_ADMIN
	case "OPERATOR":
		return authv1.Role_ROLE_OPERATOR
	case "DRIVER":
		return authv1.Role_ROLE_DRIVER
	default:
		return authv1.Role_ROLE_UNSPECIFIED
	}
}

func roleFromProto(r authv1.Role) string {
	switch r {
	case authv1.Role_ROLE_ADMIN:
		return "ADMIN"
	case authv1.Role_ROLE_OPERATOR:
		return "OPERATOR"
	case authv1.Role_ROLE_DRIVER:
		return "DRIVER"
	default:
		return ""
	}
}
