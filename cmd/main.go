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
		Service: "hyper-sync",
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
	syncService, err := wire.NewSyncService()
	if err != nil {
		return err
	}

	logger := log.FromContext(context.Background())

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
