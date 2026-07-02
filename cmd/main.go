package main

import (
	"context"
	"time"

	"butterfly.orx.me/core"
	"butterfly.orx.me/core/app"
	"butterfly.orx.me/core/log"

	"go.orx.me/apps/hyper-sync/internal/auth"
	"go.orx.me/apps/hyper-sync/internal/conf"
	"go.orx.me/apps/hyper-sync/internal/dao"
	"go.orx.me/apps/hyper-sync/internal/http"
	"go.orx.me/apps/hyper-sync/internal/post"
	"go.orx.me/apps/hyper-sync/internal/service"
	"go.orx.me/apps/hyper-sync/internal/social"
	"go.orx.me/apps/hyper-sync/internal/wire"
)

// Global API server instance

func NewApp() *app.App {
	appCore := core.New(&app.Config{
		Config:  conf.Conf,
		Service: "hypersync",
		Router:  http.Router,
		InitFunc: []func() error{
			InitIndexes,
			InitAuth,
			InitJob,
			InitPublishWorker,
			InitTokenRefresh,
		},
	})
	return appCore
}

func main() {
	app := NewApp()
	app.Run()
}

func InitAuth() error {
	logger := log.FromContext(context.Background())

	authConf := conf.Conf.Auth
	if authConf == nil {
		logger.Warn("No auth config found, using defaults")
		return nil
	}

	if authConf.Username == "" || authConf.Password == "" {
		logger.Warn("Auth username/password not configured, skipping seed")
		return nil
	}

	mongoClient := dao.NewMongoClient()
	userStore := auth.NewMongoUserStore(mongoClient, "hypersync")

	ctx := context.Background()
	if err := userStore.EnsureIndexes(ctx); err != nil {
		logger.Error("Failed to ensure user indexes", "error", err)
	}

	if err := auth.SeedUser(ctx, userStore, authConf.Username, authConf.Password); err != nil {
		logger.Error("Failed to seed user", "error", err)
		return err
	}

	return nil
}

func InitIndexes() error {
	logger := log.FromContext(context.Background())
	mongoDAO := wire.NewMongoDAO()
	if err := mongoDAO.EnsureIndexes(context.Background()); err != nil {
		logger.Error("Failed to ensure database indexes", "error", err)
	}
	return nil
}

func InitJob() error {
	logger := log.FromContext(context.Background())

	for name, social := range conf.Conf.Socials {
		if len(social.SyncTo) == 0 {
			continue
		}
		// social.Name 可能未在配置中显式设置，回退到 map key
		mainSocial := social.Name
		if mainSocial == "" {
			mainSocial = name
		}
		if err := runJob(mainSocial, social.SyncTo); err != nil {
			logger.Error("Failed to start sync job", "main_social", mainSocial, "error", err)
			return err
		}
	}

	return nil
}

// InitTokenRefresh 初始化 token 刷新定时任务
func InitTokenRefresh() error {
	logger := log.FromContext(context.Background())
	logger.Info("Initializing token refresh scheduler")

	schedulerService, err := wire.NewSchedulerService()
	if err != nil {
		logger.Error("Failed to create scheduler service", "error", err)
		return err
	}

	// 启动 token 刷新定时任务
	// 每10分钟检查一次 token 状态
	go func() {
		ctx := context.Background()
		interval := time.Minute * 10
		logger.Info("Starting token refresh scheduler", "interval", interval)
		schedulerService.StartTokenRefreshScheduler(ctx, interval)
	}()

	logger.Info("Token refresh scheduler initialized successfully")
	return nil
}

func InitPublishWorker() error {
	logger := log.FromContext(context.Background())

	mongoClient := dao.NewMongoClient()
	postStore := post.NewMongoStore(mongoClient, "hypersync")

	socialService, err := wire.NewSocialServiceOnly()
	if err != nil {
		logger.Error("Failed to create social service for publish worker", "error", err)
		return err
	}

	clients := make(map[string]social.SocialClient)
	for name, platform := range socialService.GetAllPlatforms() {
		clients[name] = platform.Client
	}

	maxRetries := 3
	if conf.Conf.Sync != nil && conf.Conf.Sync.MaxRetries > 0 {
		maxRetries = conf.Conf.Sync.MaxRetries
	}

	interval := 30 * time.Second
	if conf.Conf.Sync != nil && conf.Conf.Sync.Interval > 0 {
		interval = conf.Conf.Sync.Interval
	}

	worker := service.NewPublishWorker(postStore, clients, maxRetries)

	go func() {
		ctx := context.Background()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			if err := worker.Run(ctx); err != nil {
				logger.Error("Publish worker failed", "error", err)
			}
			<-ticker.C
		}
	}()

	logger.Info("Publish worker started", "interval", interval, "max_retries", maxRetries)
	return nil
}

func runJob(mainSocial string, socials []string) error {
	logger := log.FromContext(context.Background())
	logger.Info("Running job", "main_social", mainSocial, "socials", socials)

	syncService, err := wire.NewSyncService(mainSocial, socials)
	if err != nil {
		return err
	}

	interval := 30 * time.Second
	if conf.Conf.Sync != nil && conf.Conf.Sync.Interval > 0 {
		interval = conf.Conf.Sync.Interval
	}

	go func() {
		ctx := context.Background()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			err := syncService.Sync(ctx)
			if err != nil {
				logger.Error("Sync failed",
					"error", err)
			}
			<-ticker.C
		}
	}()

	return nil
}
