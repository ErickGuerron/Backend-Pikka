// Package grpcx agrupa los interceptores gRPC comunes de los servicios de Pikka: recuperación de
// pánicos y log estructurado por llamada (con la causa cuando falla).
package grpcx

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

// Metadata que propaga el API Gateway; el log la incluye para relacionar la llamada con su petición HTTP.
const (
	MetadataRequestID = "x-request-id"
	MetadataUserID    = "x-user-id"
)

// UnaryInterceptors recupera pánicos y registra cada llamada. Una llamada fallida registra además el
// mensaje del error, para rastrear la causa de un 500 que ve la web. Nunca registra el cuerpo de la
// petición: puede contener contraseñas o tokens.
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
			"traceId", firstValue(md, MetadataRequestID),
			"userId", firstValue(md, MetadataUserID),
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
