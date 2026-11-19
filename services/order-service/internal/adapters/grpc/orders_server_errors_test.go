package grpc

import (
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// ADR 0006: every rule of the penalty engine reaches the caller with a code it can act on, even wrapped.
func TestPenaltyErrorsMapToTheirGRPCCode(t *testing.T) {
	cases := []struct {
		err  error
		code codes.Code
	}{
		{domain.ErrCustomerBlockedBySurcharge, codes.PermissionDenied},
		{domain.ErrCustomerNotFound, codes.NotFound},
		{domain.ErrCustomerNotRecidivist, codes.FailedPrecondition},
		{domain.ErrRecidivismChargeExists, codes.Aborted},
		{domain.ErrInvalidRecidivismCharge, codes.InvalidArgument},
	}
	for _, c := range cases {
		t.Run(c.err.Error(), func(t *testing.T) {
			wantCode(t, mapOrderError(fmt.Errorf("wrapped: %w", c.err)), c.code)
		})
	}
}
