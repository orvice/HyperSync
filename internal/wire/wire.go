//go:build wireinject
// +build wireinject

package wire

import (
	"github.com/google/wire"
	"go.orx.me/apps/hyper-sync/internal/dao"
	"go.orx.me/apps/hyper-sync/internal/service"
	"go.orx.me/apps/hyper-sync/internal/social"
)

func NewSyncService(mainSocial string, socials []string) (*service.SyncService, error) {
	panic(wire.Build(
		dao.NewMongoClient,
		dao.NewPostDao,
		dao.NewSocialConfigDao,
		dao.NewSyncCursorDao,
		dao.NewThreadsConfigAdapter,
		dao.NewLocker,
		dao.NewRedisClient,
		dao.NewObjectStorage,
		service.NewSocialService,
		service.NewSyncService,
		wire.Bind(new(social.TokenManager), new(*dao.ThreadsConfigAdapter)),
	))
}

func NewSchedulerService() (*service.SchedulerService, error) {
	panic(wire.Build(
		dao.NewMongoClient,
		dao.NewSocialConfigDao,
		dao.NewSyncCursorDao,
		dao.NewThreadsConfigAdapter,
		dao.NewLocker,
		dao.NewRedisClient,
		dao.NewObjectStorage,
		service.NewSocialService,
		service.NewSchedulerService,
		wire.Bind(new(social.TokenManager), new(*dao.ThreadsConfigAdapter)),
	))
}

func NewSocialServiceOnly() (*service.SocialService, error) {
	panic(wire.Build(
		dao.NewMongoClient,
		dao.NewSocialConfigDao,
		dao.NewSyncCursorDao,
		dao.NewThreadsConfigAdapter,
		dao.NewObjectStorage,
		service.NewSocialService,
		wire.Bind(new(social.TokenManager), new(*dao.ThreadsConfigAdapter)),
	))
}

func NewMongoDAO() *dao.MongoDAO {
	panic(wire.Build(
		dao.NewMongoClient,
		dao.NewMongoDAO,
	))
}
