package server

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// LoggingInterceptor logs the method name, duration, and error (if any) for every
// unary RPC call.
func LoggingInterceptor(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	duration := time.Since(start)

	if err != nil {
		slog.Error("rpc completed",
			"method", info.FullMethod,
			"duration", duration,
			"error", err,
		)
	} else {
		slog.Info("rpc completed",
			"method", info.FullMethod,
			"duration", duration,
		)
	}

	return resp, err
}

// RecoveryInterceptor catches panics in downstream handlers, logs the stack
// trace, and returns a codes.Internal error instead of crashing the server.
func RecoveryInterceptor(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (resp any, err error) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("panic recovered in gRPC handler",
				"method", info.FullMethod,
				"panic", fmt.Sprintf("%v", r),
				"stack", string(debug.Stack()),
			)
			err = status.Errorf(codes.Internal, "internal server error")
		}
	}()
	return handler(ctx, req)
}

// AuthInterceptor returns a gRPC unary interceptor that checks the
// "authorization" metadata header for a valid Bearer token. When token is
// empty, auth is disabled and all requests pass through. The Health RPC is
// always exempt.
func AuthInterceptor(token string) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		if token == "" {
			return handler(ctx, req)
		}

		// Exempt health check.
		if info.FullMethod == "/beads.v1.BeadsService/Health" {
			return handler(ctx, req)
		}

		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		vals := md.Get("authorization")
		if len(vals) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing authorization header")
		}

		provided := vals[0]
		if !strings.HasPrefix(provided, "Bearer ") {
			return nil, status.Error(codes.Unauthenticated, "invalid authorization scheme")
		}
		provided = strings.TrimPrefix(provided, "Bearer ")

		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}

		return handler(ctx, req)
	}
}

// AuthMiddleware wraps an http.Handler and checks the Authorization header for
// a valid Bearer token. When token is empty, auth is disabled and all requests
// pass through. GET /v1/health is always exempt.
func AuthMiddleware(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Exempt health check.
		if r.Method == http.MethodGet && r.URL.Path == "/v1/health" {
			next.ServeHTTP(w, r)
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "" {
			writeError(w, http.StatusUnauthorized, "missing authorization header")
			return
		}

		if !strings.HasPrefix(auth, "Bearer ") {
			writeError(w, http.StatusUnauthorized, "invalid authorization scheme")
			return
		}

		provided := strings.TrimPrefix(auth, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		next.ServeHTTP(w, r)
	})
}
