package main

import (
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
		InitFunc: []func() error{},
	})
	return app
}

func main() {
	app := NewApp()
	app.Run()
}
