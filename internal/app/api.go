package app

import (
	"go.orx.me/apps/hyper-sync/internal/service"
)

type ApiServer struct {
	svc *service.HyperSyncService
}

func NewApiServer(service *service.HyperSyncService) (*ApiServer, error) {
	return &ApiServer{
		svc: service,
	}, nil
}
