package wire

import (
	"sync"

	"go.orx.me/apps/hyper-sync/internal/conf"
	"go.orx.me/apps/hyper-sync/internal/dao"
	"go.orx.me/apps/hyper-sync/internal/media"
	"go.orx.me/apps/hyper-sync/internal/service"
	"go.orx.me/apps/hyper-sync/internal/social"
)

var (
	socialServiceOnce     sync.Once
	socialServiceInstance  *service.SocialService
	socialServiceInitErr   error

	schedulerServiceOnce     sync.Once
	schedulerServiceInstance *service.SchedulerService
	schedulerServiceInitErr  error
)

// GetSocialService returns a process-wide singleton SocialService. This
// ensures that Telegram (and any other stateful client) only starts a
// single long-polling loop regardless of how many subsystems need the
// service.
func GetSocialService() (*service.SocialService, error) {
	socialServiceOnce.Do(func() {
		client := dao.NewMongoClient()
		socialConfigDao := dao.NewSocialConfigDao(client)
		threadsConfigAdapter := dao.NewThreadsConfigAdapter(socialConfigDao)
		syncCursorDao := dao.NewSyncCursorDao(client)

		var objectStorage media.ObjectStorage
		objectStorage, socialServiceInitErr = dao.NewObjectStorage()
		if socialServiceInitErr != nil {
			return
		}

		socialServiceInstance, socialServiceInitErr = service.NewSocialService(
			threadsConfigAdapter, syncCursorDao, objectStorage,
		)
	})
	return socialServiceInstance, socialServiceInitErr
}

// GetSocialServiceClients is a convenience helper that returns a
// name->SocialClient map from the singleton service.
func GetSocialServiceClients() (map[string]social.SocialClient, error) {
	svc, err := GetSocialService()
	if err != nil {
		return nil, err
	}
	clients := make(map[string]social.SocialClient)
	for name, platform := range svc.GetAllPlatforms() {
		clients[name] = platform.Client
	}
	return clients, nil
}

// GetSchedulerService returns a process-wide singleton SchedulerService,
// backed by the singleton SocialService.
func GetSchedulerService() (*service.SchedulerService, error) {
	schedulerServiceOnce.Do(func() {
		socialSvc, err := GetSocialService()
		if err != nil {
			schedulerServiceInitErr = err
			return
		}
		client := dao.NewMongoClient()
		socialConfigDao := dao.NewSocialConfigDao(client)
		threadsConfigAdapter := dao.NewThreadsConfigAdapter(socialConfigDao)
		locker := dao.NewLocker(dao.NewRedisClient())
		schedulerServiceInstance = service.NewSchedulerService(socialSvc, locker, threadsConfigAdapter)
	})
	return schedulerServiceInstance, schedulerServiceInitErr
}

// CDNDomain helper used by callers that need the CDN domain separately.
func CDNDomain() string {
	if conf.Conf.Storage != nil && conf.Conf.Storage.S3 != nil {
		return conf.Conf.Storage.S3.CDNDomain
	}
	return ""
}
