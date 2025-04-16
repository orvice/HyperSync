//go:build wireinject
// +build wireinject

package wire

import (
	"github.com/google/wire"
	"go.orx.me/apps/hyper-sync/internal/app"
	"go.orx.me/apps/hyper-sync/internal/dao"
)

func NewApiServer() (*app.ApiServer, error) {
	wire.Build(
		dao.NewMongoDAO,
		app.NewApiServer,
		dao.NewMongoClient,
	)
	return &app.ApiServer{}, nil
}
