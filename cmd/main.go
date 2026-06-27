package main

import (
	"context"
	"time"

	"butterfly.orx.me/core"
	"butterfly.orx.me/core/app"
	"butterfly.orx.me/core/log"

	"go.orx.me/apps/hyper-sync/internal/conf"
	"go.orx.me/apps/hyper-sync/internal/http"
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
			InitJob,
			InitTokenRefresh,
		},
	})
	return appCore
}

func main() {
	app := NewApp()
	app.Run()
}

// InitIndexes 在启动时确保数据库索引存在。
// 索引创建失败（例如存量数据中已有重复）不应阻止服务启动，仅记录错误。
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
	// 每6小时检查一次 token 状态（可以根据需要调整间隔）
	go func() {
		ctx := context.Background()
		interval := time.Minute * 10
		logger.Info("Starting token refresh scheduler", "interval", interval)
		schedulerService.StartTokenRefreshScheduler(ctx, interval)
	}()

	logger.Info("Token refresh scheduler initialized successfully")
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
