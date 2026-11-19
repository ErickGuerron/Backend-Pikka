package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	catalogv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/catalog/v1"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/catalog"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// Estas pruebas cubren la traducción de identidad y errores. Las rutas que llegan al repositorio
// (crear, listar, ajustar) se prueban en la capa de aplicación y contra PostgreSQL.
// El servicio se crea sin repositorio: las rechazadas por rol o por entrada inválida no lo tocan.
func newCatalogServer() *CatalogServer {
	return NewCatalogServer(catalog.NewService(nil, time.Now))
}

func withIdentity(role string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		metadataUserID, uuid.NewString(),
		metadataUserRole, role,
	))
}

func TestCatalogoRequiereIdentidad(t *testing.T) {
	_, err := newCatalogServer().ListProducts(context.Background(), &catalogv1.ListProductsRequest{})

	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// publicRepo sirve productos fijos al caso de uso; el resto del repositorio no se usa aquí.
type publicRepo struct {
	ports.CatalogRepository
	products []domain.Product
}

func (r publicRepo) ListActiveProducts(context.Context) ([]domain.Product, error) {
	return r.products, nil
}

func TestCatalogoPublicoSinIdentidad(t *testing.T) {
	stock := int64(7)
	repo := publicRepo{products: []domain.Product{{
		ID: uuid.New(), Name: "Pan", Description: "Recién horneado", UnitPrice: decimal.RequireFromString("2.5"),
		Active: true, StockQuantity: &stock,
	}}}
	srv := NewCatalogServer(catalog.NewService(repo, time.Now))

	res, err := srv.ListPublicProducts(context.Background(), &catalogv1.ListPublicProductsRequest{})

	require.NoError(t, err, "el catálogo público no exige identidad")
	require.Len(t, res.GetProducts(), 1)
	p := res.GetProducts()[0]
	assert.Equal(t, "Pan", p.GetName())
	assert.Equal(t, "2.50", p.GetUnitPrice())
	assert.Equal(t, "Recién horneado", p.GetDescription())
	assert.NotEmpty(t, p.GetId())
}

func TestClienteNoCreaProductos(t *testing.T) {
	_, err := newCatalogServer().CreateProduct(withIdentity("CUSTOMER"), &catalogv1.CreateProductRequest{
		Name: "Pan", UnitPrice: "1.00",
	})

	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestClienteNoPideInactivos(t *testing.T) {
	_, err := newCatalogServer().ListProducts(withIdentity("CUSTOMER"), &catalogv1.ListProductsRequest{IncludeInactive: true})

	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

func TestPrecioInvalidoEsArgumentoInvalido(t *testing.T) {
	_, err := newCatalogServer().CreateProduct(withIdentity("ADMIN"), &catalogv1.CreateProductRequest{
		Name: "Pan", UnitPrice: "abc",
	})

	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestIdDeProductoInvalido(t *testing.T) {
	_, err := newCatalogServer().GetProduct(withIdentity("ADMIN"), &catalogv1.GetProductRequest{Id: "no-es-uuid"})

	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestSoloRestockYAjusteSePuedenPedir(t *testing.T) {
	_, err := newCatalogServer().AdjustStock(withIdentity("OPERATOR"), &catalogv1.AdjustStockRequest{
		ProductId: uuid.NewString(),
		Type:      catalogv1.StockMovementType_STOCK_MOVEMENT_TYPE_SALE,
		Delta:     -1,
	})

	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}
