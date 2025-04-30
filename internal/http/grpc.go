package http

import (
	"google.golang.org/grpc"

	"go.orx.me/apps/hyper-sync/internal/wire"
	v1 "go.orx.me/apps/hyper-sync/pkg/proto/api/v1"
)

func RegisterGRPCServer(s *grpc.Server) {
	// Register auth service
	authServer, err := wire.NewAuthServer()
	if err != nil {
		panic(err)
	}
	v1.RegisterAuthServiceServer(s, authServer)
}
