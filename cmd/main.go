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

func InitJob() error {

	for _, social := range conf.Conf.Socials {
		if len(social.SyncTo) == 0 {
			continue
		}
		runJob(social.Name, social.SyncTo)
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
		interval := 6 * time.Hour
		logger.Info("Starting token refresh scheduler", "interval", interval)
		schedulerService.StartTokenRefreshScheduler(ctx, interval)
	}()

	logger.Info("Token refresh scheduler initialized successfully")
	return nil
}

func runJob(mainSocail string, socials []string) error {
	logger := log.FromContext(context.Background())
	logger.Info("Running job", "main_social", mainSocail, "socials", socials)

	syncService, err := wire.NewSyncService(mainSocail, socials)
	if err != nil {
		return err
	}

	go func() {
		for {
			err := syncService.Sync(context.Background())
			if err != nil {
				logger.Error("Sync failed",
					"error", err)
			}
			time.Sleep(30 * time.Second)
		}
	}()

	return nil
}
