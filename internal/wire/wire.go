//go:build wireinject
// +build wireinject

package wire

import (
	"github.com/google/wire"
	"go.orx.me/apps/hyper-sync/internal/dao"
	"go.orx.me/apps/hyper-sync/internal/service"
	"go.orx.me/apps/hyper-sync/internal/social"
)

func NewSyncService(mainSocail string, socials []string) (*service.SyncService, error) {
	panic(wire.Build(
		dao.NewMongoClient,
		dao.NewPostDao,
		dao.NewSocialConfigDao,
		dao.NewThreadsConfigAdapter,
		dao.NewLocker,
		dao.NewRedisClient,
		service.NewSocialService,
		service.NewSyncService,
		wire.Bind(new(social.TokenManager), new(*dao.ThreadsConfigAdapter)),
	))
}

func NewMongoDAO() *dao.MongoDAO {
	panic(wire.Build(
		dao.NewMongoClient,
		dao.NewMongoDAO,
	))
}