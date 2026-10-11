package customer

import (
	"context"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

const (
	// DefaultListLimit se usa cuando el llamador no indica limit (0).
	DefaultListLimit = 50
	// MaxListLimit acota limit; un valor mayor se limita sin error.
	MaxListLimit = 100
)

// ListCustomers devuelve una pagina de clientes con filtro opcional de estado.
type ListCustomers struct {
	repo ports.CustomerRepository
}

// NewListCustomers inyecta el repositorio.
func NewListCustomers(repo ports.CustomerRepository) *ListCustomers {
	return &ListCustomers{repo: repo}
}

// Run valida la paginacion (limit/offset negativos => ErrInvalidPagination),
// valida el status si viene (vacio no filtra; invalido => ErrInvalidStatus) y
// devuelve la pagina junto con el total que cumple el filtro.
func (uc *ListCustomers) Run(ctx context.Context, status string, limit, offset int) ([]domain.Customer, int, error) {
	if limit < 0 || offset < 0 {
		return nil, 0, domain.ErrInvalidPagination
	}
	if limit == 0 {
		limit = DefaultListLimit
	}
	if limit > MaxListLimit {
		limit = MaxListLimit
	}
	f := ports.CustomerListFilter{Limit: limit, Offset: offset}
	if status != "" {
		st := domain.CustomerStatus(status)
		if !st.Valid() {
			return nil, 0, domain.ErrInvalidStatus
		}
		f.Status = &st
	}
	return uc.repo.List(ctx, f)
}
