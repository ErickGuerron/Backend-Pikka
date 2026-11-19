package grpc

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var planInfo = &grpc.UnaryServerInfo{FullMethod: "/routing.v1.RoutingService/GeneratePlan"}

func TestLoggerRecordsTheCauseOfAFailedCall(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(metadataRequestID, "req-1"))
	failing := func(context.Context, any) (any, error) {
		return nil, status.Error(codes.Internal, "assign route 2026-11-19-z-1: stop sequences must be unique within a route")
	}

	_, err := logger(log)(ctx, nil, planInfo, failing)

	if status.Code(err) != codes.Internal {
		t.Fatalf("code = %v, want Internal (the interceptor must not change the error)", status.Code(err))
	}
	out := buf.String()
	for _, want := range []string{`"level":"ERROR"`, "stop sequences must be unique", `"traceId":"req-1"`, "GeneratePlan"} {
		if !strings.Contains(out, want) {
			t.Fatalf("log %q does not contain %q", out, want)
		}
	}
}

func TestLoggerWarnsOnRejectedCalls(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	rejected := func(context.Context, any) (any, error) {
		return nil, status.Error(codes.FailedPrecondition, "no driver available for route")
	}

	_, _ = logger(log)(context.Background(), nil, planInfo, rejected)

	if out := buf.String(); !strings.Contains(out, `"level":"WARN"`) || !strings.Contains(out, "no driver available") {
		t.Fatalf("log = %q, want a WARN with the reason", out)
	}
}

func TestRecovererTurnsAPanicIntoInternal(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	panics := func(context.Context, any) (any, error) { panic("boom") }

	_, err := recoverer(log)(context.Background(), nil, planInfo, panics)

	if status.Code(err) != codes.Internal || !strings.Contains(buf.String(), "panic in grpc handler") {
		t.Fatalf("err = %v, log = %q; want Internal and a logged panic", err, buf.String())
	}
}
