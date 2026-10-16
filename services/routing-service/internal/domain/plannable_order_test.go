package domain_test

import (
	"errors"
	"testing"

	"github.com/ErickGuerron/Backend-Pikka/services/routing-service/internal/domain"
)

func TestPlannableOrderValidate(t *testing.T) {
	tests := []struct {
		name    string
		order   domain.PlannableOrder
		wantErr error
	}{
		{"valid", domain.PlannableOrder{OrderID: "o-1", ZoneID: "z-1"}, nil},
		{"empty order id", domain.PlannableOrder{OrderID: "", ZoneID: "z-1"}, domain.ErrInvalidOrder},
		{"whitespace order id", domain.PlannableOrder{OrderID: "  \t", ZoneID: "z-1"}, domain.ErrInvalidOrder},
		{"empty zone id", domain.PlannableOrder{OrderID: "o-1", ZoneID: ""}, domain.ErrInvalidOrder},
		{"whitespace zone id", domain.PlannableOrder{OrderID: "o-1", ZoneID: " "}, domain.ErrInvalidOrder},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.order.Validate()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Validate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
