// Package grpc expone los casos de uso del dominio Customer del Order Service
// mediante gRPC.
//
// Es la mitad servidor de BE-T5 (la mitad REST vive en BE-T5b). El server
// solo traduce: parsea la entrada, delega en los casos de uso de
// internal/application/customer y mapea los errores de dominio a códigos
// gRPC. No contiene reglas de negocio.
//
// Por ADR 0003 el handler NO llama a auth-service para verificar el rol: la
// autorización por rol + alcance (x-user-id / x-user-role en metadata) se
// resuelve en los interceptores del gateway y los use cases ya la aplican
// donde corresponde (ADR 0008 §3).
package grpc

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	customerv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/customer/v1"
	customercase "github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/customer"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// UseCases agrupa los casos de uso del dominio Customer que el adaptador
// gRPC necesita. Cada campo es opcional (puntero) para que el caller pueda
// inyectar solo los que tenga implementados; si un RPC llega a un caso de
// uso no cableado devolvemos codes.Unimplemented, igual que auth-service.
type UseCases struct {
	Create          *customercase.CreateCustomer
	GetByID         *customercase.GetByID
	GetByUserID     *customercase.GetByUserID
	List            *customercase.ListCustomers
	UpdateStatus    *customercase.UpdateStatus
	CanCreateOrders *customercase.CanCreateOrders
}

// CustomerServer implementa customerv1.CustomerServiceServer.
//
// Embedding UnimplementedCustomerServiceServer garantiza compatibilidad
// forward: si agregamos rpcs nuevas en el .proto, los handlers viejos
// siguen compilando.
type CustomerServer struct {
	customerv1.UnimplementedCustomerServiceServer
	useCases UseCases
}

// NewCustomerServer inyecta los use cases (constructor injection).
func NewCustomerServer(uc UseCases) *CustomerServer {
	return &CustomerServer{useCases: uc}
}

// CreateCustomer da de alta un Customer nuevo a partir de un user_id.
func (s *CustomerServer) CreateCustomer(ctx context.Context, req *customerv1.CreateCustomerRequest) (*customerv1.CreateCustomerResponse, error) {
	if s.useCases.Create == nil {
		return nil, status.Error(codes.Unimplemented, "create customer is not wired")
	}
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "user_id inválido: %v", err)
	}
	c, err := s.useCases.Create.Run(ctx, userID, req.GetEmail(), req.GetFullName(), req.GetPhone(), req.GetCity())
	if err != nil {
		return nil, mapError(err)
	}
	return &customerv1.CreateCustomerResponse{Customer: domainToProto(c)}, nil
}

// GetByUserID devuelve el Customer asociado a un user_id.
func (s *CustomerServer) GetByUserID(ctx context.Context, req *customerv1.GetByUserIDRequest) (*customerv1.GetByUserIDResponse, error) {
	if s.useCases.GetByUserID == nil {
		return nil, status.Error(codes.Unimplemented, "get by user id is not wired")
	}
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "user_id inválido: %v", err)
	}
	c, err := s.useCases.GetByUserID.Run(ctx, userID)
	if err != nil {
		return nil, mapError(err)
	}
	return &customerv1.GetByUserIDResponse{Customer: domainToProto(c)}, nil
}

// GetByID devuelve el Customer por su id interno.
func (s *CustomerServer) GetByID(ctx context.Context, req *customerv1.GetByIDRequest) (*customerv1.GetByIDResponse, error) {
	if s.useCases.GetByID == nil {
		return nil, status.Error(codes.Unimplemented, "get by id is not wired")
	}
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id inválido: %v", err)
	}
	c, err := s.useCases.GetByID.Run(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}
	return &customerv1.GetByIDResponse{Customer: domainToProto(c)}, nil
}

// ListCustomers devuelve una pagina de Customers con filtro opcional de estado.
func (s *CustomerServer) ListCustomers(ctx context.Context, req *customerv1.ListCustomersRequest) (*customerv1.ListCustomersResponse, error) {
	if s.useCases.List == nil {
		return nil, status.Error(codes.Unimplemented, "list customers is not wired")
	}
	cs, total, err := s.useCases.List.Run(ctx, req.GetStatus(), int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]*customerv1.Customer, 0, len(cs))
	for _, c := range cs {
		out = append(out, domainToProto(c))
	}
	return &customerv1.ListCustomersResponse{Customers: out, Total: int32(total)}, nil
}

// UpdateStatus cambia el estado del Customer. El handler valida el status
// antes de delegar para devolver codes.InvalidArgument con un mensaje claro.
func (s *CustomerServer) UpdateStatus(ctx context.Context, req *customerv1.UpdateStatusRequest) (*customerv1.UpdateStatusResponse, error) {
	if s.useCases.UpdateStatus == nil {
		return nil, status.Error(codes.Unimplemented, "update status is not wired")
	}
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "id inválido: %v", err)
	}
	st := domain.CustomerStatus(req.GetStatus())
	if !st.Valid() {
		return nil, status.Errorf(codes.InvalidArgument, "status inválido: %q", req.GetStatus())
	}
	c, err := s.useCases.UpdateStatus.Run(ctx, id, st)
	if err != nil {
		return nil, mapError(err)
	}
	return &customerv1.UpdateStatusResponse{Customer: domainToProto(c)}, nil
}

// CanCreateOrders informa si el cliente puede crear pedidos. Delega en el
// caso de uso (BE-T4b), que ya concentra la regla de negocio.
func (s *CustomerServer) CanCreateOrders(ctx context.Context, req *customerv1.CanCreateOrdersRequest) (*customerv1.CanCreateOrdersResponse, error) {
	if s.useCases.CanCreateOrders == nil {
		return nil, status.Error(codes.Unimplemented, "can create orders is not wired")
	}
	id, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "user_id inválido: %v", err)
	}
	can, err := s.useCases.CanCreateOrders.Run(ctx, id)
	if err != nil {
		return nil, mapError(err)
	}
	return &customerv1.CanCreateOrdersResponse{CanCreate: can}, nil
}

// mapError traduce los sentinel errors de dominio a códigos gRPC.
// Es la tabla de traducción del adaptador: si el dominio agrega un error
// nuevo, este switch es el único punto a tocar.
func mapError(err error) error {
	switch {
	case errors.Is(err, domain.ErrCustomerNotFound):
		return status.Error(codes.NotFound, "cliente no encontrado")
	case errors.Is(err, domain.ErrCustomerAlreadyExists):
		return status.Error(codes.AlreadyExists, "cliente ya existe")
	case errors.Is(err, domain.ErrInvalidPagination):
		return status.Error(codes.InvalidArgument, "paginación inválida")
	case errors.Is(err, domain.ErrInvalidStatus):
		return status.Error(codes.InvalidArgument, "status inválido")
	case errors.Is(err, domain.ErrInvalidPhone), errors.Is(err, domain.ErrInvalidCity):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "deadline exceeded")
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "canceled")
	default:
		// El detalle queda en el log del interceptor; al cliente no se le filtra.
		return status.Error(codes.Internal, "internal error")
	}
}

// domainToProto traduce una entidad de dominio al mensaje protobuf.
// Mantenerlo en una función aparte permite cambiar el wire format (p.ej.
// fechas) en un solo lugar.
func domainToProto(c domain.Customer) *customerv1.Customer {
	out := &customerv1.Customer{
		Id:               c.ID.String(),
		UserId:           c.UserID.String(),
		FullName:         c.FullName,
		Email:            c.Email,
		Phone:            c.Phone,
		City:             c.City,
		Status:           string(c.Status),
		PendingSurcharge: c.PendingSurcharge.StringFixed(2),
		CreatedAt:        c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:        c.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if !c.RecidivistAt.IsZero() {
		out.RecidivistAt = c.RecidivistAt.UTC().Format(time.RFC3339)
	}
	return out
}
