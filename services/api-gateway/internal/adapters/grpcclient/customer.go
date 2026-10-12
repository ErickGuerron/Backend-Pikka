// Package grpcclient crea clientes gRPC hacia los servicios internos.
package grpcclient

import (
	"context"
	"time"

	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"

	customerv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/customer/v1"
)

// Customer envuelve al cliente del Order Service exponiendo el dominio
// Customer. Impone deadline y propaga la metadata de identidad del Gateway
// (x-user-id / x-user-role / x-request-id).
type Customer struct {
	client  customerv1.CustomerServiceClient
	health  healthpb.HealthClient
	timeout time.Duration
}

// NewCustomer crea el envoltorio sobre una conexión gRPC existente.
func NewCustomer(conn grpc.ClientConnInterface, timeout time.Duration) *Customer {
	return &Customer{
		client:  customerv1.NewCustomerServiceClient(conn),
		health:  healthpb.NewHealthClient(conn),
		timeout: timeout,
	}
}

// NewCustomerWithClient permite inyectar un cliente falso en pruebas.
func NewCustomerWithClient(client customerv1.CustomerServiceClient, timeout time.Duration) *Customer {
	return &Customer{client: client, timeout: timeout}
}

// outgoing aplica timeout y propaga metadata al contexto de la llamada.
// Misma forma que Auth y Zones para no introducir una variante local.
func (c *Customer) outgoing(ctx context.Context, m CallMeta) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	pairs := []string{MetadataRequestID, m.RequestID}
	if m.UserID != "" {
		pairs = append(pairs, MetadataUserID, m.UserID, MetadataUserRole, m.Role)
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...), cancel
}

// GetByUserID devuelve el Customer asociado a un user_id del esquema auth.
// Lo consume `GET /api/v1/customers/me`.
func (c *Customer) GetByUserID(ctx context.Context, m CallMeta, req *customerv1.GetByUserIDRequest) (*customerv1.GetByUserIDResponse, error) {
	ctx, cancel := c.outgoing(ctx, m)
	defer cancel()
	return c.client.GetByUserID(ctx, req)
}

// CreateCustomer registra el Customer en el Order Service. Lo invoca el
// Gateway tras un signup exitoso (ruta publica: m sin identidad).
func (c *Customer) CreateCustomer(ctx context.Context, m CallMeta, req *customerv1.CreateCustomerRequest) (*customerv1.CreateCustomerResponse, error) {
	ctx, cancel := c.outgoing(ctx, m)
	defer cancel()
	return c.client.CreateCustomer(ctx, req)
}

// GetByID devuelve un Customer por su id interno (solo ADMIN en el HTTP).
func (c *Customer) GetByID(ctx context.Context, m CallMeta, req *customerv1.GetByIDRequest) (*customerv1.GetByIDResponse, error) {
	ctx, cancel := c.outgoing(ctx, m)
	defer cancel()
	return c.client.GetByID(ctx, req)
}

// ListCustomers devuelve una pagina de Customers (solo ADMIN en el HTTP).
func (c *Customer) ListCustomers(ctx context.Context, m CallMeta, req *customerv1.ListCustomersRequest) (*customerv1.ListCustomersResponse, error) {
	ctx, cancel := c.outgoing(ctx, m)
	defer cancel()
	return c.client.ListCustomers(ctx, req)
}

// UpdateStatus cambia el estado del Customer. El handler HTTP solo lo
// expone para ADMIN; el use case revalida el rol como defensa en profundidad.
func (c *Customer) UpdateStatus(ctx context.Context, m CallMeta, req *customerv1.UpdateStatusRequest) (*customerv1.UpdateStatusResponse, error) {
	ctx, cancel := c.outgoing(ctx, m)
	defer cancel()
	return c.client.UpdateStatus(ctx, req)
}

// Ready verifica con el health protocol que el servicio responde. Devuelve
// true si el cliente no fue inicializado con health check (caso de los
// tests con cliente falso).
func (c *Customer) Ready(ctx context.Context) bool {
	if c.health == nil {
		return true
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	res, err := c.health.Check(ctx, &healthpb.HealthCheckRequest{Service: customerv1.CustomerService_ServiceDesc.ServiceName})
	return err == nil && res.GetStatus() == healthpb.HealthCheckResponse_SERVING
}
