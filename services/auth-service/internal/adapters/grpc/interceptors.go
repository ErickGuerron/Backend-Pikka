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

// UnaryInterceptors devuelve recuperación de pánicos y log estructurado por llamada.
// Nunca registra el cuerpo de la petición: puede contener contraseñas o tokens.
func UnaryInterceptors(log *slog.Logger) grpc.ServerOption {
	return grpc.ChainUnaryInterceptor(recoverer(log), logger(log))
}

func logger(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		md, _ := metadata.FromIncomingContext(ctx)
		attrs := []any{
			"method", info.FullMethod,
			"code", status.Code(err).String(),
			"durationMs", time.Since(start).Milliseconds(),
			"traceId", firstValue(md, MetadataRequestID),
			"userId", firstValue(md, MetadataUserID),
		}
		if status.Code(err) == codes.Internal {
			log.ErrorContext(ctx, "grpc request failed", attrs...)
		} else {
			log.InfoContext(ctx, "grpc request", attrs...)
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
