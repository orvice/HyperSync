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
