package grpc_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	zonesv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/zones/v1"
	grpcadapter "github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/adapters/grpc"
	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/application"
	"github.com/ErickGuerron/Backend-Pikka/services/zone-service/internal/domain"
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

// ListCoverageZones abre la cobertura a todo rol autenticado, pero ListZones sigue cerrada.
func TestListCoverageZones_AnyAuthenticatedRoleSeesOnlyActiveZonesWithoutSettings(t *testing.T) {
	srv := grpcadapter.NewServer(application.NewZoneService(&memRepo{zones: map[string]domain.Zone{
		"a": {ID: "a", Code: "NORTE", Name: "Zona Norte", Status: domain.StatusActive, CreatedBy: "admin-1", Settings: domain.Settings{DepotName: "Hub Norte"}},
		"b": {ID: "b", Code: "SUR", Name: "Zona Sur", Status: domain.StatusInactive, StatusReason: "Obras"},
	}}))

	for _, role := range []string{"ADMIN", "OPERATOR", "CUSTOMER", "DRIVER", "SERVICE"} {
		res, err := srv.ListCoverageZones(asRole(role), &zonesv1.ListCoverageZonesRequest{})
		if err != nil {
			t.Fatalf("role %s: %v", role, err)
		}
		if len(res.GetZones()) != 1 || res.GetZones()[0].GetStatus() != "ACTIVE" {
			t.Fatalf("role %s: want only the active zone, got %v", role, res.GetZones())
		}
		if z := res.GetZones()[0]; z.GetSettings() != nil || z.GetCreatedBy() != "" || z.GetUpdatedBy() != "" {
			t.Fatalf("role %s: operational data leaked: %v", role, z)
		}
	}
}

func TestListCoverageZones_RequiresIdentityAndListZonesStaysClosedForCustomer(t *testing.T) {
	srv := newServer()
	_, err := srv.ListCoverageZones(context.Background(), &zonesv1.ListCoverageZonesRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code = %s, want UNAUTHENTICATED", status.Code(err))
	}
	_, err = srv.ListZones(asRole("CUSTOMER"), &zonesv1.ListZonesRequest{})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("code = %s, want PERMISSION_DENIED", status.Code(err))
	}
}
