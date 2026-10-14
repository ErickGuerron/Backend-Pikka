package grpc_test

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	zonesv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/zones/v1"
)

// La identidad interna SERVICE solo puede clasificar puntos (Order Service valida pedidos).
// Cualquier otra operación de zonas sigue reservada a ADMIN y OPERATOR.
func TestClassifyPoint_ServiceIdentityIsAllowed(t *testing.T) {
	_, err := newServer().ClassifyPoint(asRole("SERVICE"), &zonesv1.ClassifyPointRequest{
		Point: &zonesv1.Coordinate{Latitude: -0.18, Longitude: -78.48},
	})
	// Sin zonas registradas en el fake el resultado es NOT_FOUND; lo importante es no recibir PERMISSION_DENIED.
	if status.Code(err) == codes.PermissionDenied || status.Code(err) == codes.Unauthenticated {
		t.Fatalf("SERVICE identity rejected: %v", err)
	}
}

func TestClassifyPoint_UserRolesOtherThanStaffStillDenied(t *testing.T) {
	for _, role := range []string{"CUSTOMER", "DRIVER"} {
		_, err := newServer().ClassifyPoint(asRole(role), &zonesv1.ClassifyPointRequest{
			Point: &zonesv1.Coordinate{Latitude: -0.18, Longitude: -78.48},
		})
		if status.Code(err) != codes.PermissionDenied {
			t.Fatalf("role %s: code = %s, want PERMISSION_DENIED", role, status.Code(err))
		}
	}
}

func TestListZones_ServiceIdentityIsNotAllowed(t *testing.T) {
	_, err := newServer().ListZones(asRole("SERVICE"), nil)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %s, want PERMISSION_DENIED", status.Code(err))
	}
}
