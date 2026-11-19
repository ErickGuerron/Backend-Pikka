package order

import (
	"context"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/application/ports"
	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// ListRecidivists gives the operator the customers an extra charge can be applied to (ADR 0006). The
// list is the operator's: no other role reaches the repository.
type ListRecidivists struct {
	repo ports.RecidivismRepository
}

func NewListRecidivists(repo ports.RecidivismRepository) *ListRecidivists {
	return &ListRecidivists{repo: repo}
}

func (uc *ListRecidivists) Run(ctx context.Context, role string) ([]domain.Recidivist, error) {
	if !isOperator(role) {
		return nil, domain.ErrNotOperator
	}
	return uc.repo.ListRecidivists(ctx)
}
