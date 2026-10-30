package grpc

import (
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// Crear y cancelar con la cuenta bloqueada por saldo llegan al Gateway como PERMISSION_DENIED (403),
// tambien cuando el error viene envuelto.
func TestMapOrderErrorBalanceBlockedIsPermissionDenied(t *testing.T) {
	for _, err := range []error{domain.ErrCustomerBlockedBySurcharge, fmt.Errorf("create: %w", domain.ErrCustomerBlockedBySurcharge)} {
		wantCode(t, mapOrderError(err), codes.PermissionDenied)
	}
}
