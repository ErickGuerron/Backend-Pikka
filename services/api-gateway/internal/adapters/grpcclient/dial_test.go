package grpcclient

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

const (
	// reconnectDownTime simulates an upstream container being recreated: the
	// old address refuses connections for this long.
	reconnectDownTime = 17 * time.Second
	// reconnectDeadline is the maximum time the client may take to return to
	// READY once the upstream listens again on the same address.
	reconnectDeadline = 8 * time.Second
)

// TestDialReconnects guards against the stale-address reconnect stall: after an
// upstream is restarted on the same address, the Dial connection must recover
// within reconnectDeadline instead of backing off for minutes.
func TestDialReconnects(t *testing.T) {
	if testing.Short() {
		t.Skip("slow reconnect test (~25s); run without -short")
	}

	addr := freeLoopbackAddr(t)
	srv := startHealthServer(t, addr)

	conn, err := Dial(addr)
	if err != nil {
		t.Fatalf("Dial(%s): %v", addr, err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	waitForState(t, conn, connectivity.Ready, 10*time.Second)

	srv.Stop()
	waitUntilLeavesReady(t, conn, 5*time.Second)
	probeDuring(conn, reconnectDownTime)

	startHealthServer(t, addr)

	restartedAt := time.Now()
	waitForState(t, conn, connectivity.Ready, reconnectDeadline)
	t.Logf("reconnected %s after upstream restart", time.Since(restartedAt).Round(time.Millisecond))
}

// freeLoopbackAddr returns a 127.0.0.1 address that was free at the time of the call.
func freeLoopbackAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve free port: %v", err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatalf("release free port: %v", err)
	}
	return addr
}

// startHealthServer serves the standard gRPC health service on addr and stops it on cleanup.
// It retries the bind briefly so a just-released port can be reused.
func startHealthServer(t *testing.T, addr string) *grpc.Server {
	t.Helper()
	var lis net.Listener
	var err error
	deadline := time.Now().Add(5 * time.Second)
	for {
		lis, err = net.Listen("tcp", addr)
		if err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("listen on %s: %v", addr, err)
	}

	srv := grpc.NewServer()
	healthpb.RegisterHealthServer(srv, health.NewServer())
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return srv
}

// waitForState drives the connection until it reaches want or the timeout expires.
func waitForState(t *testing.T, conn *grpc.ClientConn, want connectivity.State, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn.Connect()
	for {
		state := conn.GetState()
		if state == want {
			return
		}
		if state == connectivity.Idle {
			conn.Connect()
		}
		if !conn.WaitForStateChange(ctx, state) {
			t.Fatalf("connection state is %s after %s; want %s", conn.GetState(), timeout, want)
		}
	}
}

// probeDuring sends health RPCs while the upstream is down, the way gateway traffic
// does. A dropped connection sits in IDLE until an RPC asks for a new attempt; the
// failed attempts then enter TRANSIENT_FAILURE and follow the connect backoff.
func probeDuring(conn *grpc.ClientConn, d time.Duration) {
	client := healthpb.NewHealthClient(conn)
	end := time.Now().Add(d)
	for time.Now().Before(end) {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		_, _ = client.Check(ctx, &healthpb.HealthCheckRequest{})
		cancel()
		time.Sleep(500 * time.Millisecond)
	}
}

// waitUntilLeavesReady confirms the stop was observed by the client before the downtime starts.
func waitUntilLeavesReady(t *testing.T, conn *grpc.ClientConn, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for conn.GetState() == connectivity.Ready {
		if !conn.WaitForStateChange(ctx, connectivity.Ready) {
			t.Fatalf("connection stayed READY %s after upstream stop", timeout)
		}
	}
}
