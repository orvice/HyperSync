//go:build wireinject
// +build wireinject

package wire

import (
	"github.com/google/wire"
	"go.orx.me/apps/hyper-sync/internal/app"
	"go.orx.me/apps/hyper-sync/internal/conf"
	"go.orx.me/apps/hyper-sync/internal/dao"
	"go.orx.me/apps/hyper-sync/internal/server"
	"go.orx.me/apps/hyper-sync/internal/service" // Add service package import
)

func NewApiServer() (*app.ApiServer, error) {
	wire.Build(
		// DAO Layer
		dao.NewMongoClient, // Provides *mongo.Client
		dao.NewMongoDAO,    // Provides *MongoDAO, needs *mongo.Client
		dao.NewUserDAO,     // Provides UserDAO, needs *MongoDAO
		dao.NewRedisClient,
		conf.Get,

		// Service Layer
		service.NewAuthService,      // Needs UserDAO
		service.NewHyperSyncService, // Needs ??? (Add dependencies if any)

		// App Layer
		app.NewApiServer, // Needs *AuthService, *HyperSyncService, etc.
	)
	return &app.ApiServer{}, nil
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
