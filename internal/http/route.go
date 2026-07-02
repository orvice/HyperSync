package http

import (
	"connectrpc.com/connect"
	"github.com/gin-gonic/gin"

	"go.orx.me/apps/hyper-sync/internal/auth"
	"go.orx.me/apps/hyper-sync/internal/conf"
	"go.orx.me/apps/hyper-sync/internal/dao"
	"go.orx.me/apps/hyper-sync/internal/handler"
	"go.orx.me/apps/hyper-sync/internal/media"
	"go.orx.me/apps/hyper-sync/internal/post"
	"go.orx.me/apps/hyper-sync/internal/service"
	"go.orx.me/apps/hyper-sync/internal/social"
	"go.orx.me/apps/hyper-sync/internal/wire"
	"go.orx.me/apps/hyper-sync/pkg/proto/api/v1/v1connect"
)

func Router(r *gin.Engine) {

	// Routes
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	// ConnectRPC services
	mountConnectRPC(r)

	// API routes
	api := r.Group("/api")
	{
		// Token management routes
		tokenRoutes := api.Group("/token")
		{
			schedulerService, err := wire.NewSchedulerService()
			if err != nil {
				panic(err)
			}

			tokenHandler := handler.NewTokenHandler(schedulerService)

			tokenRoutes.GET("/status/:platform", tokenHandler.GetTokenStatus)
			tokenRoutes.POST("/refresh/:platform", tokenHandler.RefreshToken)
			tokenRoutes.POST("/refresh-all", tokenHandler.RefreshAllTokens)
		}
	}
}

func mountConnectRPC(r *gin.Engine) {
	authConf := conf.Conf.Auth
	if authConf == nil {
		authConf = &conf.AuthConfig{
			Username:  "admin",
			Password:  "admin",
			JWTSecret: "change-me-in-production",
		}
	}

	mongoClient := dao.NewMongoClient()
	userStore := auth.NewMongoUserStore(mongoClient, "hypersync")
	interceptor := auth.NewAuthInterceptor(authConf.JWTSecret)

	authService := service.NewAuthService(userStore, authConf.JWTSecret)
	authPath, authHandler := v1connect.NewAuthServiceHandler(authService, connect.WithInterceptors(interceptor))
	r.Any(authPath+"*path", gin.WrapH(authHandler))

	postStore := post.NewMongoStore(mongoClient, "hypersync")

	// Build platform deleter from social clients
	var postOpts []service.PostServiceOption
	if socialService, err := wire.NewSocialServiceOnly(); err == nil {
		clients := make(map[string]social.SocialClient)
		for name, platform := range socialService.GetAllPlatforms() {
			clients[name] = platform.Client
		}
		postOpts = append(postOpts, service.WithPlatformDeleter(service.NewSocialPlatformDeleter(clients)))
	}

	postService := service.NewPostService(postStore, postOpts...)
	postPath, postHandler := v1connect.NewPostServiceHandler(postService, connect.WithInterceptors(interceptor))
	r.Any(postPath+"*path", gin.WrapH(postHandler))

	// Media service
	mediaStore := media.NewMongoStore(mongoClient, "hypersync")
	var objectStorage media.ObjectStorage
	cdnDomain := ""
	if conf.Conf.Storage != nil && conf.Conf.Storage.S3 != nil {
		s3Conf := conf.Conf.Storage.S3
		objectStorage = media.NewS3ObjectStorage(media.S3Config{
			Endpoint:  s3Conf.Endpoint,
			Bucket:    s3Conf.Bucket,
			AccessKey: s3Conf.AccessKey,
			SecretKey: s3Conf.SecretKey,
			Region:    s3Conf.Region,
		})
		cdnDomain = s3Conf.CDNDomain
	} else {
		objectStorage = media.NewMemoryObjectStorage()
	}

	mediaService := service.NewMediaService(mediaStore, objectStorage, cdnDomain)
	mediaPath, mediaHandler := v1connect.NewMediaServiceHandler(mediaService, connect.WithInterceptors(interceptor))
	r.Any(mediaPath+"*path", gin.WrapH(mediaHandler))
	r.POST("/api/media/upload", gin.WrapF(mediaService.HandleUpload))
}
