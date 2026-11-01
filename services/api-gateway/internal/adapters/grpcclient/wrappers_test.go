package grpcclient

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	catalogv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/catalog/v1"
	routingv1 "github.com/ErickGuerron/Backend-Pikka/contracts/gen/go/routing/v1"
)

// recordedCall es una llamada unaria capturada por recordingConn.
type recordedCall struct {
	method      string
	md          metadata.MD
	hasDeadline bool
}

// recordingConn implementa grpc.ClientConnInterface para los clientes generados:
// registra cada Invoke (método, metadata saliente y si hubo deadline), devuelve un
// error fijo y, para el health check, un estado fijo. No toca la red.
type recordingConn struct {
	err    error
	health healthpb.HealthCheckResponse_ServingStatus
	calls  []recordedCall
}

func (c *recordingConn) Invoke(ctx context.Context, method string, _ any, reply any, _ ...grpc.CallOption) error {
	md, _ := metadata.FromOutgoingContext(ctx)
	_, hasDeadline := ctx.Deadline()
	c.calls = append(c.calls, recordedCall{method: method, md: md, hasDeadline: hasDeadline})
	if hc, ok := reply.(*healthpb.HealthCheckResponse); ok {
		hc.Status = c.health
	}
	return c.err
}

func (c *recordingConn) NewStream(context.Context, *grpc.StreamDesc, string, ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, errors.New("streaming is not used by these wrappers")
}

// wrapperFn invoca un wrapper de grpcclient con una petición vacía.
type wrapperFn func(ctx context.Context, m CallMeta) error

// rpc adapta un método de wrapper a wrapperFn. La petición no se inspecciona:
// el wrapper solo la reenvía, así que se usa nil.
func rpc[Req, Resp any](fn func(context.Context, CallMeta, *Req) (*Resp, error)) wrapperFn {
	return func(ctx context.Context, m CallMeta) error {
		_, err := fn(ctx, m, nil)
		return err
	}
}

func authCalls(cc grpc.ClientConnInterface) map[string]wrapperFn {
	a := NewAuth(cc, time.Second)
	return map[string]wrapperFn{
		"Login":          rpc(a.Login),
		"SignupCustomer": rpc(a.SignupCustomer),
		"CreateUser":     rpc(a.CreateUser),
		"GetUser":        rpc(a.GetUser),
		"ListUsers":      rpc(a.ListUsers),
		"UpdateUser":     rpc(a.UpdateUser),
		"DeactivateUser": rpc(a.DeactivateUser),
	}
}

func catalogCalls(cc grpc.ClientConnInterface) map[string]wrapperFn {
	c := NewCatalog(cc, time.Second)
	return map[string]wrapperFn{
		"ListProducts":       rpc(c.ListProducts),
		"ListPublicProducts": rpc(c.ListPublicProducts),
		"GetProduct":         rpc(c.GetProduct),
		"CreateProduct":      rpc(c.CreateProduct),
		"SetProductActive":   rpc(c.SetProductActive),
		"AdjustStock":        rpc(c.AdjustStock),
		"ListStockMovements": rpc(c.ListStockMovements),
	}
}

func customerCalls(cc grpc.ClientConnInterface) map[string]wrapperFn {
	c := NewCustomer(cc, time.Second)
	return map[string]wrapperFn{
		"GetByUserID":    rpc(c.GetByUserID),
		"CreateCustomer": rpc(c.CreateCustomer),
		"GetByID":        rpc(c.GetByID),
		"ListCustomers":  rpc(c.ListCustomers),
		"UpdateStatus":   rpc(c.UpdateStatus),
	}
}

func driversCalls(cc grpc.ClientConnInterface) map[string]wrapperFn {
	d := NewDrivers(cc, time.Second)
	return map[string]wrapperFn{
		"GetDriverByUserID":  rpc(d.GetDriverByUserID),
		"GetDriverByRouteID": rpc(d.GetDriverByRouteID),
		"CreateDriver":       rpc(d.CreateDriver),
		"RegisterDelivery":   rpc(d.RegisterDelivery),
		"StartRoute":         rpc(d.StartRoute),
		"ListDrivers":        rpc(d.ListDrivers),
	}
}

func ordersCalls(cc grpc.ClientConnInterface) map[string]wrapperFn {
	o := NewOrders(cc, time.Second)
	return map[string]wrapperFn{
		"CreateOrder":                       rpc(o.CreateOrder),
		"ListMyOrders":                      rpc(o.ListMyOrders),
		"ListMyNotifications":               rpc(o.ListMyNotifications),
		"ListOrderExceptions":               rpc(o.ListOrderExceptions),
		"ReviewOrder":                       rpc(o.ReviewOrder),
		"ListSurchargesPendingVerification": rpc(o.ListSurchargesPendingVerification),
		"ResolveSurchargeVerification":      rpc(o.ResolveSurchargeVerification),
		"ApplyRecidivismCharge":             rpc(o.ApplyRecidivismCharge),
		"ClearRecidivist":                   rpc(o.ClearRecidivist),
		"ListRecidivists":                   rpc(o.ListRecidivists),
		"PayMyBalance":                      rpc(o.PayMyBalance),
		"GetMyBalance":                      rpc(o.GetMyBalance),
		"CancelOrder":                       rpc(o.CancelOrder),
		"GetMyOrder":                        rpc(o.GetMyOrder),
		"StopsBeforeForOrder":               rpc(o.StopsBeforeForOrder),
		"AdvanceRouteStop":                  rpc(o.AdvanceRouteStop),
		"RecordRouteStopOutcome":            rpc(o.RecordRouteStopOutcome),
		"ListDriverRouteStops":              rpc(o.ListDriverRouteStops),
		"ListOrders":                        rpc(o.ListOrders),
		"SetReassignmentFee":                rpc(o.SetReassignmentFee),
		"GetReassignmentFee":                rpc(o.GetReassignmentFee),
		"ApplyReassignmentCharge":           rpc(o.ApplyReassignmentCharge),
		"OpenDispute":                       rpc(o.OpenDispute),
		"ListOpenDisputes":                  rpc(o.ListOpenDisputes),
		"ResolveDispute":                    rpc(o.ResolveDispute),
	}
}

func routingCalls(cc grpc.ClientConnInterface) map[string]wrapperFn {
	r := NewRouting(cc, time.Second)
	return map[string]wrapperFn{
		"GeneratePlan": rpc(r.GeneratePlan),
	}
}

func trackingCalls(cc grpc.ClientConnInterface) map[string]wrapperFn {
	t := NewTracking(cc, time.Second)
	return map[string]wrapperFn{
		"EstimateArrival":   rpc(t.EstimateArrival),
		"GetDriverPosition": rpc(t.GetDriverPosition),
		"RecordPosition":    rpc(t.RecordPosition),
	}
}

func zonesCalls(cc grpc.ClientConnInterface) map[string]wrapperFn {
	z := NewZones(cc, time.Second)
	return map[string]wrapperFn{
		"CreateZone":        rpc(z.CreateZone),
		"GetZone":           rpc(z.GetZone),
		"ListZones":         rpc(z.ListZones),
		"ListCoverageZones": rpc(z.ListCoverageZones),
		"UpdateZone":        rpc(z.UpdateZone),
		"SetZoneStatus":     rpc(z.SetZoneStatus),
		"DeleteZone":        rpc(z.DeleteZone),
		"ClassifyPoint":     rpc(z.ClassifyPoint),
	}
}

// wrapperSets agrupa los wrappers por servicio para recorrer todos con la misma tabla.
var wrapperSets = map[string]func(grpc.ClientConnInterface) map[string]wrapperFn{
	"Auth":     authCalls,
	"Catalog":  catalogCalls,
	"Customer": customerCalls,
	"Drivers":  driversCalls,
	"Orders":   ordersCalls,
	"Routing":  routingCalls,
	"Tracking": trackingCalls,
	"Zones":    zonesCalls,
}

// Cada wrapper reenvía la identidad del Gateway, aplica deadline y devuelve el error del servicio sin cambiarlo.
func TestUnaryWrappersForwardIdentityDeadlineAndServiceError(t *testing.T) {
	injected := status.Error(codes.PermissionDenied, "service says no")
	meta := CallMeta{RequestID: "req-7", UserID: "u-9", Role: "ADMIN"}

	for svcName, build := range wrapperSets {
		for rpcName := range build(nil) {
			t.Run(svcName+"/"+rpcName, func(t *testing.T) {
				conn := &recordingConn{err: injected}

				err := build(conn)[rpcName](context.Background(), meta)

				assert.ErrorIs(t, err, injected)
				assert.Equal(t, codes.PermissionDenied, status.Code(err))
				require.Len(t, conn.calls, 1)
				got := conn.calls[0]
				assert.True(t, strings.HasSuffix(got.method, "/"+rpcName), "method %q", got.method)
				assert.True(t, got.hasDeadline, "call must carry the configured timeout")
				assert.Equal(t, []string{"req-7"}, got.md.Get(MetadataRequestID))
				assert.Equal(t, []string{"u-9"}, got.md.Get(MetadataUserID))
				assert.Equal(t, []string{"ADMIN"}, got.md.Get(MetadataUserRole))
			})
		}
	}
}

// Sin usuario autenticado (ruta publica) solo viaja el request id; no se inventan cabeceras de identidad.
func TestUnaryWrappersOmitIdentityWithoutUser(t *testing.T) {
	meta := CallMeta{RequestID: "req-anon"}

	for svcName, build := range wrapperSets {
		for rpcName := range build(nil) {
			t.Run(svcName+"/"+rpcName, func(t *testing.T) {
				conn := &recordingConn{}

				require.NoError(t, build(conn)[rpcName](context.Background(), meta))

				require.Len(t, conn.calls, 1)
				md := conn.calls[0].md
				assert.Equal(t, []string{"req-anon"}, md.Get(MetadataRequestID))
				assert.Empty(t, md.Get(MetadataUserID))
				assert.Empty(t, md.Get(MetadataUserRole))
			})
		}
	}
}

// Ready solo confirma al servicio cuando el health check responde SERVING.
func TestReadyWrappersReportServingOnly(t *testing.T) {
	unavailable := status.Error(codes.Unavailable, "down")
	cases := []struct {
		name   string
		err    error
		health healthpb.HealthCheckResponse_ServingStatus
		want   bool
	}{
		{"sirviendo", nil, healthpb.HealthCheckResponse_SERVING, true},
		{"no sirve", nil, healthpb.HealthCheckResponse_NOT_SERVING, false},
		{"desconocido", nil, healthpb.HealthCheckResponse_UNKNOWN, false},
		{"inalcanzable", unavailable, healthpb.HealthCheckResponse_SERVING, false},
	}
	readers := map[string]func(grpc.ClientConnInterface) bool{
		"Auth": func(cc grpc.ClientConnInterface) bool {
			return NewAuth(cc, time.Second).Ready(context.Background())
		},
		"Customer": func(cc grpc.ClientConnInterface) bool {
			return NewCustomer(cc, time.Second).Ready(context.Background())
		},
	}

	for svcName, ready := range readers {
		for _, tc := range cases {
			t.Run(svcName+"/"+tc.name, func(t *testing.T) {
				conn := &recordingConn{err: tc.err, health: tc.health}

				assert.Equal(t, tc.want, ready(conn))
				require.Len(t, conn.calls, 1)
				assert.Equal(t, "/grpc.health.v1.Health/Check", conn.calls[0].method)
			})
		}
	}
}

// Los constructores con cliente inyectado envuelven el cliente generado sin cambiar su comportamiento.
func TestWithClientConstructorsForwardCallsToTheInjectedClient(t *testing.T) {
	conn := &recordingConn{}
	meta := CallMeta{RequestID: "req-inj", UserID: "u-1", Role: "ADMIN"}

	_, err := NewCatalogWithClient(catalogv1.NewCatalogServiceClient(conn), time.Second).ListProducts(context.Background(), meta, &catalogv1.ListProductsRequest{})
	require.NoError(t, err)
	_, err = NewRoutingWithClient(routingv1.NewRoutingServiceClient(conn), time.Second).GeneratePlan(context.Background(), meta, &routingv1.GeneratePlanRequest{})
	require.NoError(t, err)

	require.Len(t, conn.calls, 2)
	assert.True(t, strings.HasSuffix(conn.calls[0].method, "/ListProducts"), conn.calls[0].method)
	assert.True(t, strings.HasSuffix(conn.calls[1].method, "/GeneratePlan"), conn.calls[1].method)
	assert.Equal(t, []string{"u-1"}, conn.calls[1].md.Get(MetadataUserID))
}

// Sin health check configurado (clientes falsos de prueba) Ready devuelve true sin llamar a la red.
func TestReadyWithoutHealthClientIsAlwaysTrue(t *testing.T) {
	ctx := context.Background()

	assert.True(t, NewAuthWithClient(nil, time.Second).Ready(ctx))
	assert.True(t, NewCustomerWithClient(nil, time.Second).Ready(ctx))
	assert.True(t, (&Zones{}).Ready(ctx))
}
