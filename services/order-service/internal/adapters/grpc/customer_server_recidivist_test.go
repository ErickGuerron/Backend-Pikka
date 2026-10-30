package grpc

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ErickGuerron/Backend-Pikka/services/order-service/internal/domain"
)

// The operator needs to see who is a recidivist before applying the extra charge.
func TestDomainToProtoCarriesTheRecidivistMark(t *testing.T) {
	marked := time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC)
	got := domainToProto(domain.Customer{ID: uuid.New(), UserID: uuid.New(), RecidivistAt: marked})
	if got.GetRecidivistAt() != "2026-10-10T09:00:00Z" {
		t.Fatalf("recidivist_at = %q", got.GetRecidivistAt())
	}
}

func TestDomainToProtoLeavesTheMarkEmptyForAnOrdinaryCustomer(t *testing.T) {
	got := domainToProto(domain.Customer{ID: uuid.New(), UserID: uuid.New()})
	if got.GetRecidivistAt() != "" {
		t.Fatalf("recidivist_at = %q, want empty", got.GetRecidivistAt())
	}
}
