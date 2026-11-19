package grpc

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Metadata that the Gateway propagates and that the log carries to correlate a call with its HTTP request.
const (
	metadataRequestID = "x-request-id"
	metadataUserID    = "x-user-id"
)

// UnaryInterceptors recovers from panics and logs every call. A failed call also logs the error
// message, so a 500 seen by the web can be traced to its cause (for example a route rejected by
// Order Service). Request bodies are never logged.
func UnaryInterceptors(log *slog.Logger) grpc.ServerOption {
	return grpc.ChainUnaryInterceptor(recoverer(log), logger(log))
}

func logger(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		md, _ := metadata.FromIncomingContext(ctx)
		code := status.Code(err)
		attrs := []any{
			"method", info.FullMethod,
			"code", code.String(),
			"durationMs", time.Since(start).Milliseconds(),
			"traceId", firstValue(md, metadataRequestID),
			"userId", firstValue(md, metadataUserID),
		}
		switch code {
		case codes.OK:
			log.InfoContext(ctx, "grpc request", attrs...)
		case codes.Internal, codes.Unknown, codes.DataLoss:
			log.ErrorContext(ctx, "grpc request failed", append(attrs, "error", status.Convert(err).Message())...)
		default:
			log.WarnContext(ctx, "grpc request rejected", append(attrs, "error", status.Convert(err).Message())...)
		}
		return resp, err
	}
}

func recoverer(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				log.ErrorContext(ctx, "panic in grpc handler", "method", info.FullMethod, "panic", r, "stack", string(debug.Stack()))
				err = status.Error(codes.Internal, "internal error")
			}
		}()
		return handler(ctx, req)
	}
}

func firstValue(md metadata.MD, key string) string {
	if v := md.Get(key); len(v) > 0 {
		return v[0]
	}
	return ""
}
