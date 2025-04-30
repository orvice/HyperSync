package main

import (
	"context"
	"log"
	"time"

	"butterfly.orx.me/core"
	"butterfly.orx.me/core/app"
	"go.orx.me/apps/hyper-sync/internal/conf"
	"go.orx.me/apps/hyper-sync/internal/http"
)

func NewApp() *app.App {
	app := core.New(&app.Config{
		Config:   conf.Conf,
		Service:  "hyper-sync",
		Router:   http.Router,
		InitFunc: []func() error{initSyncJob},
	})
	return app
}

// initSyncJob initializes and starts the post sync job
func initSyncJob() error {
	log.Println("Initializing post sync job")

	// Get service providers from the app context
	postService := http.GetPostService()
	if postService == nil {
		return nil // Services not initialized yet
	}

	// Start the sync job with a 15-minute interval
	err := postService.StartSyncJob(context.Background(), 15*time.Minute)
	if err != nil {
		log.Printf("Failed to start post sync job: %v", err)
		return err
	}

	log.Println("Post sync job initialized successfully")
	return nil
}

func main() {
	app := NewApp()
	app.Run()
}
