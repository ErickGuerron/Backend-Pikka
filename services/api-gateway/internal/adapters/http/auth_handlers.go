package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/auth/v1"
	customerv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/customer/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/api-gateway/internal/adapters/grpcclient"
)

// AuthClient es lo que los handlers necesitan del Auth Service.
type AuthClient interface {
	Login(ctx context.Context, m grpcclient.CallMeta, req *authv1.LoginRequest) (*authv1.LoginResponse, error)
	CreateUser(ctx context.Context, m grpcclient.CallMeta, req *authv1.CreateUserRequest) (*authv1.CreateUserResponse, error)
	GetUser(ctx context.Context, m grpcclient.CallMeta, req *authv1.GetUserRequest) (*authv1.GetUserResponse, error)
	SignupCustomer(ctx context.Context, m grpcclient.CallMeta, req *authv1.SignupCustomerRequest) (*authv1.SignupCustomerResponse, error)
	ListUsers(ctx context.Context, m grpcclient.CallMeta, req *authv1.ListUsersRequest) (*authv1.ListUsersResponse, error)
	UpdateUser(ctx context.Context, m grpcclient.CallMeta, req *authv1.UpdateUserRequest) (*authv1.UpdateUserResponse, error)
	DeactivateUser(ctx context.Context, m grpcclient.CallMeta, req *authv1.DeactivateUserRequest) (*authv1.DeactivateUserResponse, error)
}

// Los handlers solo traducen REST <-> gRPC. La autorización y las reglas viven en los servicios.
type authHandlers struct {
	auth      AuthClient
	customers CustomerClient // opcional: si es nil el signup no crea Customer
}

// CustomerClient es lo que el signup necesita del Order Service.
type CustomerClient interface {
	CreateCustomer(ctx context.Context, m grpcclient.CallMeta, req *customerv1.CreateCustomerRequest) (*customerv1.CreateCustomerResponse, error)
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

type signupRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"fullName"`
}

type signupResponse struct {
	AccessToken string  `json:"accessToken"`
	TokenType   string  `json:"tokenType"`
	ExpiresIn   int64   `json:"expiresIn"`
	User        userDTO `json:"user"`
}

// updateUserRequest: los campos ausentes (nil) no cambian. Un rol inválido o
// CUSTOMER llega como ROLE_UNSPECIFIED y auth-service lo rechaza con 400.
type updateUserRequest struct {
	FullName *string `json:"fullName"`
	Role     *string `json:"role"`
}

type listUsersResponse struct {
	Users []userDTO `json:"users"`
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

// signup registra un cliente nuevo. Ruta pública: no propaga identidad del
// Gateway (el cliente todavía no existe). Devuelve 201 con el JWT.
func (h *authHandlers) signup(c echo.Context) error {
	var req signupRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	res, err := h.auth.SignupCustomer(c.Request().Context(), callMeta(c), &authv1.SignupCustomerRequest{
		Email:    req.Email,
		Password: req.Password,
		FullName: req.FullName,
	})
	if err != nil {
		return err
	}
	h.createCustomer(c, res.GetUser())
	return c.JSON(http.StatusCreated, signupResponse{
		AccessToken: res.GetAccessToken(),
		TokenType:   res.GetTokenType(),
		ExpiresIn:   res.GetExpiresInSeconds(),
		User:        toUserDTO(res.GetUser()),
	})
}

// createCustomer registra el Customer en order-service tras un signup exitoso.
//
// Política de fallos: la cuenta de auth ya existe y el cliente recibe su token,
// así que un fallo aquí NO se propaga: se loguea con el request id y el signup
// responde 201. AlreadyExists cuenta como éxito (idempotencia). Un único
// reintento cubre cortes transitorios (Unavailable). Si igualmente falla, el
// reintento posterior es seguro: CreateCustomer es idempotente por user_id
// (ErrCustomerAlreadyExists -> AlreadyExists), por lo que un backfill o job de
// reconciliación puede volver a llamarlo sin riesgo.
func (h *authHandlers) createCustomer(c echo.Context, u *authv1.User) {
	if h.customers == nil || u == nil {
		return
	}
	req := &customerv1.CreateCustomerRequest{UserId: u.GetId(), Email: u.GetEmail(), FullName: u.GetFullName()}
	m := callMeta(c)
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		_, err = h.customers.CreateCustomer(c.Request().Context(), m, req)
		code := status.Code(err)
		if err == nil || code == codes.AlreadyExists {
			return
		}
		if code != codes.Unavailable {
			break
		}
	}
	c.Logger().Errorf("signup: create customer failed request_id=%s user_id=%s err=%v", m.RequestID, u.GetId(), err)
}

// listUsers devuelve la lista completa de usuarios. La autorización por rol
// (solo ADMIN) la aplica el adminOnlyMiddleware antes de llegar a este handler;
// la capa de auth-service revalida por defensa en profundidad.
func (h *authHandlers) listUsers(c echo.Context) error {
	res, err := h.auth.ListUsers(c.Request().Context(), callMeta(c), &authv1.ListUsersRequest{})
	if err != nil {
		return err
	}
	out := listUsersResponse{Users: make([]userDTO, 0, len(res.GetUsers()))}
	for _, u := range res.GetUsers() {
		out.Users = append(out.Users, toUserDTO(u))
	}
	return c.JSON(http.StatusOK, out)
}

// updateUser edita nombre y/o rol de un usuario (solo ADMIN, filtrado por adminOnly).
func (h *authHandlers) updateUser(c echo.Context) error {
	var req updateUserRequest
	if err := bindJSON(c, &req); err != nil {
		return err
	}
	in := &authv1.UpdateUserRequest{Id: c.Param("id"), FullName: req.FullName}
	if req.Role != nil {
		role := roleToProto(*req.Role)
		in.Role = &role
	}
	res, err := h.auth.UpdateUser(c.Request().Context(), callMeta(c), in)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, toUserDTO(res.GetUser()))
}

// deactivateUser hace borrado lógico (active=false) y devuelve el usuario (solo ADMIN).
func (h *authHandlers) deactivateUser(c echo.Context) error {
	res, err := h.auth.DeactivateUser(c.Request().Context(), callMeta(c), &authv1.DeactivateUserRequest{Id: c.Param("id")})
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, toUserDTO(res.GetUser()))
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
	case authv1.Role_ROLE_CUSTOMER:
		return "CUSTOMER"
	default:
		return ""
	}
}
