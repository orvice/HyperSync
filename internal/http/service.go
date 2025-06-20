package http

import (
	"sync"
	"time"

	"go.orx.me/apps/hyper-sync/internal/conf"
	"go.orx.me/apps/hyper-sync/internal/dao"
	"go.orx.me/apps/hyper-sync/internal/service"
)

var (
	postService      *service.PostService
	socialService    *service.SocialService
	syncService      *service.SyncService
	schedulerService *service.SchedulerService
	webhookService   *service.WebhookService
	mongoDAO         *dao.MongoDAO
	serviceInitOnce  sync.Once
	serviceInitDone  bool
)

// InitServices initializes all services
func InitServices(dao *dao.MongoDAO) {
	serviceInitOnce.Do(func() {
		mongoDAO = dao

		// Initialize social service with configurations from the conf package
		social, err := service.NewSocialService(conf.Conf.Socials)
		if err != nil {
			panic(err)
		}
		socialService = social

		// Initialize post service
		postService = service.NewPostService(dao, socialService)

		// Initialize sync service with default configuration
		syncConfig := &service.SyncConfig{
			MaxRetries:      3,
			SyncInterval:    15 * time.Minute,
			BatchSize:       20,
			MaxMemosPerRun:  100,
			TargetPlatforms: []string{"mastodon", "bluesky"}, // Default platforms
			MemosConfig: &service.MemosConfig{
				Endpoint: "", // Will be set from config
				Token:    "", // Will be set from config
			},
			SkipPrivate: true,
			SkipOlder:   7 * 24 * time.Hour, // Skip memos older than 7 days
		}

		// TODO: Load config from conf.Conf or environment variables
		sync, err := service.NewSyncService(dao, socialService, syncConfig)
		if err == nil {
			syncService = sync
		}

		// Initialize scheduler service
		schedulerConfig := &service.SchedulerConfig{
			AutoSyncEnabled:    false, // Disabled by default
			DefaultInterval:    15 * time.Minute,
			MaxConcurrentTasks: 3,
			MaxRetries:         3,
			RetryDelay:         5 * time.Minute,
			QueueSize:          100,
			TaskTimeout:        10 * time.Minute,
		}
		if syncService != nil {
			schedulerService = service.NewSchedulerService(syncService, dao, schedulerConfig)
		}

		// Initialize webhook service
		webhookConfig := &service.WebhookConfig{
			Enabled:        false, // Disabled by default
			AllowedSources: []string{"memos", "github", "manual"},
			Timeout:        30 * time.Second,
		}
		if schedulerService != nil {
			webhookService = service.NewWebhookService(schedulerService, webhookConfig)
		}

		serviceInitDone = true
	})
}

// GetPostService returns the post service instance
func GetPostService() *service.PostService {
	if !serviceInitDone {
		return nil
	}
	return postService
}

// GetSocialService returns the social service instance
func GetSocialService() *service.SocialService {
	if !serviceInitDone {
		return nil
	}
	return socialService
}

// GetSyncService returns the sync service instance
func GetSyncService() *service.SyncService {
	if !serviceInitDone {
		return nil
	}
	return syncService
}

// GetMongoDAO returns the MongoDB DAO instance
func GetMongoDAO() *dao.MongoDAO {
	if !serviceInitDone {
		return nil
	}
	return mongoDAO
}

// GetSchedulerService returns the scheduler service instance
func GetSchedulerService() *service.SchedulerService {
	if !serviceInitDone {
		return nil
	}
	return schedulerService
}

// GetWebhookService returns the webhook service instance
func GetWebhookService() *service.WebhookService {
	if !serviceInitDone {
		return nil
	}
	return webhookService
}
