package main

import (
	"butterfly.orx.me/core"
	"butterfly.orx.me/core/app"

	"go.orx.me/apps/hyper-sync/internal/conf"
	"go.orx.me/apps/hyper-sync/internal/http"
)

// Global API server instance

func NewApp() *app.App {
	appCore := core.New(&app.Config{
		Config:   conf.Conf,
		Service:  "hyper-sync",
		Router:   http.Router,
		InitFunc: []func() error{},
	})
	return appCore
}

func main() {
	app := NewApp()
	app.Run()
}
