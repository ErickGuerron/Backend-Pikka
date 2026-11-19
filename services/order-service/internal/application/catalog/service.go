// Package catalog contiene los casos de uso del catálogo de productos (ADR 0009).
package catalog

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// ErrForbidden indica que el rol no puede hacer la operación. El adaptador la traduce a PERMISSION_DENIED.
var ErrForbidden = errors.New("role cannot manage the catalog")

// Actor es quien pide la operación, con la identidad propagada por el Gateway.
type Actor struct {
	UserID uuid.UUID
	Role   string
}

// Service coordina el catálogo. Las reglas de rol viven aquí, no en el transporte.
type Service struct {
	repo ports.CatalogRepository
	now  func() time.Time
}

// NewService crea el caso de uso. now se inyecta para que las pruebas controlen el reloj.
func NewService(repo ports.CatalogRepository, now func() time.Time) *Service {
	return &Service{repo: repo, now: now}
}

func isStaff(role string) bool {
	return role == "ADMIN" || role == "OPERATOR"
}

// ListProducts devuelve el catálogo. El cliente solo ve productos activos; pedir los inactivos es
// una operación de personal.
func (s *Service) ListProducts(ctx context.Context, actor Actor, includeInactive bool) ([]domain.Product, error) {
	if !isStaff(actor.Role) {
		if includeInactive {
			return nil, ErrForbidden
		}
		return s.repo.ListActiveProducts(ctx)
	}
	if includeInactive {
		return s.repo.ListAllProducts(ctx)
	}
	return s.repo.ListActiveProducts(ctx)
}

// publicProductsLimit acota la vista pública de la landing.
// shortcut: sin paginación, se devuelven los primeros 50 por nombre; añadir cursor cuando la landing lo necesite.
const publicProductsLimit = 50

// ListPublicProducts devuelve los productos activos para la landing, sin identidad de usuario.
func (s *Service) ListPublicProducts(ctx context.Context) ([]domain.Product, error) {
	list, err := s.repo.ListActiveProducts(ctx)
	if err != nil {
		return nil, err
	}
	if len(list) > publicProductsLimit {
		list = list[:publicProductsLimit]
	}
	return list, nil
}

// GetProduct devuelve un producto. Para el cliente, un producto inactivo es como si no existiera.
func (s *Service) GetProduct(ctx context.Context, actor Actor, id uuid.UUID) (domain.Product, error) {
	p, err := s.repo.GetProduct(ctx, id)
	if err != nil {
		return domain.Product{}, err
	}
	if !isStaff(actor.Role) && !p.Active {
		return domain.Product{}, domain.ErrProductNotFound
	}
	return p, nil
}

// CreateProduct valida y guarda un producto nuevo, con su entrada de stock inicial si la hay.
func (s *Service) CreateProduct(ctx context.Context, actor Actor, input domain.NewProductInput) (domain.Product, error) {
	if !isStaff(actor.Role) {
		return domain.Product{}, ErrForbidden
	}
	now := s.now()
	product, movement, err := domain.NewProduct(uuid.New(), input, &actor.UserID, now)
	if err != nil {
		return domain.Product{}, err
	}
	if err := s.repo.CreateProduct(ctx, product, movement); err != nil {
		return domain.Product{}, err
	}
	return product, nil
}

// SetProductActive oferta o retira un producto.
func (s *Service) SetProductActive(ctx context.Context, actor Actor, id uuid.UUID, active bool) (domain.Product, error) {
	if !isStaff(actor.Role) {
		return domain.Product{}, ErrForbidden
	}
	if err := s.repo.SetActive(ctx, id, active, s.now()); err != nil {
		return domain.Product{}, err
	}
	return s.repo.GetProduct(ctx, id)
}

// AdjustStock registra una entrada o un ajuste manual. Los movimientos SALE y RETURN los genera el
// servicio al pedir o cancelar, así que no se aceptan aquí.
func (s *Service) AdjustStock(ctx context.Context, actor Actor, productID uuid.UUID, kind domain.MovementType, delta int64, reason string) (domain.StockMovement, error) {
	if !isStaff(actor.Role) {
		return domain.StockMovement{}, ErrForbidden
	}
	if kind != domain.MovementRestock && kind != domain.MovementAdjustment {
		return domain.StockMovement{}, domain.ErrInvalidMovement
	}
	return s.repo.AdjustStock(ctx, productID, kind, delta, reason, &actor.UserID, s.now())
}

// ListStockMovements devuelve el historial de un producto. Es información interna: solo personal.
func (s *Service) ListStockMovements(ctx context.Context, actor Actor, productID uuid.UUID) ([]domain.StockMovement, error) {
	if !isStaff(actor.Role) {
		return nil, ErrForbidden
	}
	if _, err := s.repo.GetProduct(ctx, productID); err != nil {
		return nil, err
	}
	return s.repo.ListMovements(ctx, productID)
}

// ParsePrice convierte el precio enviado como texto ("2.50") en decimal, sin pasar por float.
func ParsePrice(raw string) (decimal.Decimal, error) {
	price, err := decimal.NewFromString(raw)
	if err != nil {
		return decimal.Decimal{}, domain.ErrInvalidPrice
	}
	return price, nil
}
