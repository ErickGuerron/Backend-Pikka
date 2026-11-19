package grpcclient

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	catalogv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/catalog/v1"
)

// Catalog envuelve al cliente de CatalogService del Order Service, que sirve en el mismo puerto
// gRPC que los pedidos. Impone deadline y propaga la identidad del Gateway, igual que Orders.
type Catalog struct {
	client  catalogv1.CatalogServiceClient
	timeout time.Duration
}

// NewCatalog crea el envoltorio sobre una conexión gRPC existente.
func NewCatalog(conn grpc.ClientConnInterface, timeout time.Duration) *Catalog {
	return &Catalog{client: catalogv1.NewCatalogServiceClient(conn), timeout: timeout}
}

// NewCatalogWithClient permite inyectar un cliente falso en pruebas.
func NewCatalogWithClient(client catalogv1.CatalogServiceClient, timeout time.Duration) *Catalog {
	return &Catalog{client: client, timeout: timeout}
}

func (c *Catalog) outgoing(ctx context.Context, m CallMeta) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	pairs := []string{MetadataRequestID, m.RequestID}
	if m.UserID != "" {
		pairs = append(pairs, MetadataUserID, m.UserID, MetadataUserRole, m.Role)
	}
	return metadata.AppendToOutgoingContext(ctx, pairs...), cancel
}

// ListProducts devuelve el catálogo. El servicio decide si el rol puede pedir inactivos.
func (c *Catalog) ListProducts(ctx context.Context, m CallMeta, req *catalogv1.ListProductsRequest) (*catalogv1.ListProductsResponse, error) {
	ctx, cancel := c.outgoing(ctx, m)
	defer cancel()
	return c.client.ListProducts(ctx, req)
}

// ListPublicProducts devuelve el catálogo público (sin identidad de usuario).
func (c *Catalog) ListPublicProducts(ctx context.Context, m CallMeta, req *catalogv1.ListPublicProductsRequest) (*catalogv1.ListPublicProductsResponse, error) {
	ctx, cancel := c.outgoing(ctx, m)
	defer cancel()
	return c.client.ListPublicProducts(ctx, req)
}

// GetProduct devuelve un producto.
func (c *Catalog) GetProduct(ctx context.Context, m CallMeta, req *catalogv1.GetProductRequest) (*catalogv1.GetProductResponse, error) {
	ctx, cancel := c.outgoing(ctx, m)
	defer cancel()
	return c.client.GetProduct(ctx, req)
}

// CreateProduct crea un producto (personal).
func (c *Catalog) CreateProduct(ctx context.Context, m CallMeta, req *catalogv1.CreateProductRequest) (*catalogv1.CreateProductResponse, error) {
	ctx, cancel := c.outgoing(ctx, m)
	defer cancel()
	return c.client.CreateProduct(ctx, req)
}

// SetProductActive oferta o retira un producto (personal).
func (c *Catalog) SetProductActive(ctx context.Context, m CallMeta, req *catalogv1.SetProductActiveRequest) (*catalogv1.SetProductActiveResponse, error) {
	ctx, cancel := c.outgoing(ctx, m)
	defer cancel()
	return c.client.SetProductActive(ctx, req)
}

// AdjustStock registra una entrada o un ajuste manual (personal).
func (c *Catalog) AdjustStock(ctx context.Context, m CallMeta, req *catalogv1.AdjustStockRequest) (*catalogv1.AdjustStockResponse, error) {
	ctx, cancel := c.outgoing(ctx, m)
	defer cancel()
	return c.client.AdjustStock(ctx, req)
}

// ListStockMovements devuelve el historial de un producto (personal).
func (c *Catalog) ListStockMovements(ctx context.Context, m CallMeta, req *catalogv1.ListStockMovementsRequest) (*catalogv1.ListStockMovementsResponse, error) {
	ctx, cancel := c.outgoing(ctx, m)
	defer cancel()
	return c.client.ListStockMovements(ctx, req)
}
