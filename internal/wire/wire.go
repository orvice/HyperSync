//go:build wireinject
// +build wireinject

package wire

import (
	"github.com/google/wire"
	"go.orx.me/apps/hyper-sync/internal/dao"
	"go.orx.me/apps/hyper-sync/internal/service"
)

func NewSyncService() (*service.SyncService, error) {
	panic(wire.Build(
		dao.NewMongoClient,
		dao.NewMongoDAO,
		service.NewSyncService,
	))
}
