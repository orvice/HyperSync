package app

import (
	"go.orx.me/apps/hyper-sync/internal/service"
)

type ApiServer struct {
	svc     *service.HyperSyncService
	authSvc *service.AuthService // Add AuthService field
}

func NewApiServer(
	svc *service.HyperSyncService,
	authSvc *service.AuthService, // Add AuthService parameter
) (*ApiServer, error) {
	return &ApiServer{
		svc:     svc,
		authSvc: authSvc, // Assign AuthService
	}, nil
}
