//go:build wireinject
// +build wireinject

package wire

import (
	"github.com/google/wire"
	"go.orx.me/apps/hyper-sync/internal/conf"
	"go.orx.me/apps/hyper-sync/internal/dao"
	"go.orx.me/apps/hyper-sync/internal/server"
	"go.orx.me/apps/hyper-sync/internal/service"
	"go.orx.me/apps/hyper-sync/internal/social"
)

// ProvideConfig provides the platform configuration from the global config
func ProvideConfig() map[string]*social.PlatformConfig {
	return conf.Get().Socials
}

func NewAuthServer() (*server.AuthServer, error) {
	wire.Build(
		dao.NewMongoClient,
		dao.NewMongoDAO,
		dao.NewUserDAO,
		dao.NewRedisClient,

		service.NewAuthService,
		server.NewAuthServer,
		conf.Get,
	)
	return &server.AuthServer{}, nil
}

func NewPostServer() (*server.PostServer, error) {
	wire.Build(
		dao.NewMongoClient,
		dao.NewMongoDAO,

		ProvideConfig,
		service.NewSocialService,
		service.NewPostService,
		server.NewPostServer,
	)
	return &server.PostServer{}, nil
}
