package server

import (
	beadsv1 "github.com/groblegark/kbeads/gen/beads/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// NewGRPCServer creates a gRPC server with standard interceptors,
// registers the BeadsService, reflection, and returns the server ready to serve.
// When authToken is non-empty, an auth interceptor is inserted between
// Recovery and Logging.
func NewGRPCServer(beadsServer *BeadsServer, authToken string) *grpc.Server {
	interceptors := []grpc.UnaryServerInterceptor{RecoveryInterceptor}
	if authToken != "" {
		interceptors = append(interceptors, AuthInterceptor(authToken))
	}
	interceptors = append(interceptors, LoggingInterceptor)

	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(interceptors...),
	)

	beadsv1.RegisterBeadsServiceServer(srv, beadsServer)
	reflection.Register(srv)

	return srv
}
