package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"butterfly.orx.me/core"
	"butterfly.orx.me/core/app"
	"butterfly.orx.me/core/log"

	"go.orx.me/apps/hyper-sync/internal/auth"
	"go.orx.me/apps/hyper-sync/internal/conf"
	"go.orx.me/apps/hyper-sync/internal/dao"
	"go.orx.me/apps/hyper-sync/internal/http"
	"go.orx.me/apps/hyper-sync/internal/media"
	"go.orx.me/apps/hyper-sync/internal/post"
	"go.orx.me/apps/hyper-sync/internal/service"
	"go.orx.me/apps/hyper-sync/internal/wire"
	"go.orx.me/apps/hyper-sync/internal/worker"
)

// shutdownCtx is cancelled on SIGINT/SIGTERM; every background worker loop
// derives from it. Butterfly core exposes no shutdown hook (TeardownFunc is
// never invoked), so the process owns its own signal handling. Set in main
// before app.Run invokes the Init funcs.
var shutdownCtx context.Context

// workerWG tracks running worker loops so main can drain them on shutdown.
var workerWG sync.WaitGroup

// drainTimeout bounds the shutdown wait for in-flight worker iterations. The
// publish worker caps per-post work at 2 minutes; this leaves headroom.
const drainTimeout = 3 * time.Minute

// startWorker runs fn immediately and then on every interval tick until
// shutdown, registering the loop with workerWG so main can drain it.
func startWorker(interval time.Duration, fn func(context.Context)) {
	workerWG.Add(1)
	go func() {
		defer workerWG.Done()
		worker.RunLoop(shutdownCtx, interval, fn)
	}()
}

func NewApp() *app.App {
	appCore := core.New(&app.Config{
		Config:  conf.Conf,
		Service: "hypersync",
		Router:  http.Router,
		InitFunc: []func() error{
			InitIndexes,
			InitAuth,
			InitJob,
			InitPublishWorker,
			InitTokenRefresh,
		},
	})
	return appCore
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	shutdownCtx = ctx

	go func() {
		<-ctx.Done()
		stop() // restore default handling so a second signal kills immediately

		logger := log.FromContext(context.Background())
		logger.Info("Shutdown signal received, draining background workers")

		done := make(chan struct{})
		go func() {
			workerWG.Wait()
			close(done)
		}()

		select {
		case <-done:
			logger.Info("Background workers stopped")
		case <-time.After(drainTimeout):
			logger.Warn("Timed out waiting for background workers, exiting anyway")
		}
		os.Exit(0)
	}()

	app := NewApp()
	app.Run()
}

func InitAuth() error {
	logger := log.FromContext(context.Background())

	authConf := conf.Conf.Auth
	if authConf == nil || authConf.JWTSecret == "" {
		return errors.New("auth.jwt_secret must be configured; refusing to start with a forgeable JWT secret")
	}

	if authConf.Username == "" || authConf.Password == "" {
		return errors.New("auth.username and auth.password must be configured to seed the initial user")
	}

	mongoClient := dao.NewMongoClient()
	userStore := auth.NewMongoUserStore(mongoClient, "hypersync")

	ctx := context.Background()
	if err := userStore.EnsureIndexes(ctx); err != nil {
		logger.Error("Failed to ensure user indexes", "error", err)
	}

	if err := auth.SeedUser(ctx, userStore, authConf.Username, authConf.Password); err != nil {
		logger.Error("Failed to seed user", "error", err)
		return err
	}

	return nil
}

func InitIndexes() error {
	logger := log.FromContext(context.Background())
	mongoDAO := wire.NewMongoDAO()
	if err := mongoDAO.EnsureIndexes(context.Background()); err != nil {
		logger.Error("Failed to ensure database indexes", "error", err)
	}
	return nil
}

func InitJob() error {
	logger := log.FromContext(context.Background())

	for name, social := range conf.Conf.Socials {
		if len(social.SyncTo) == 0 {
			continue
		}
		// social.Name 可能未在配置中显式设置，回退到 map key
		mainSocial := social.Name
		if mainSocial == "" {
			mainSocial = name
		}
		if err := runJob(mainSocial, social.SyncTo); err != nil {
			logger.Error("Failed to start sync job", "main_social", mainSocial, "error", err)
			return err
		}
	}

	return nil
}

// InitTokenRefresh 初始化 token 刷新定时任务
func InitTokenRefresh() error {
	logger := log.FromContext(context.Background())
	logger.Info("Initializing token refresh scheduler")

	schedulerService, err := wire.GetSchedulerService()
	if err != nil {
		logger.Error("Failed to create scheduler service", "error", err)
		return err
	}

	// 启动 token 刷新定时任务
	// 每10分钟检查一次 token 状态
	// StartTokenRefreshScheduler 自带 ticker 循环并在 ctx.Done() 时退出
	workerWG.Add(1)
	go func() {
		defer workerWG.Done()
		interval := time.Minute * 10
		logger.Info("Starting token refresh scheduler", "interval", interval)
		schedulerService.StartTokenRefreshScheduler(shutdownCtx, interval)
	}()

	logger.Info("Token refresh scheduler initialized successfully")
	return nil
}

func InitPublishWorker() error {
	logger := log.FromContext(context.Background())

	mongoClient := dao.NewMongoClient()
	postStore := post.NewMongoStore(mongoClient, "hypersync")
	if err := postStore.EnsureIndexes(context.Background()); err != nil {
		logger.Error("Failed to ensure managed post indexes", "error", err)
	}
	mediaStore := media.NewMongoStore(mongoClient, "hypersync")

	clients, err := wire.GetSocialServiceClients()
	if err != nil {
		logger.Error("Failed to get social service for publish worker", "error", err)
		return err
	}

	maxRetries := 3
	if conf.Conf.Sync != nil && conf.Conf.Sync.MaxRetries > 0 {
		maxRetries = conf.Conf.Sync.MaxRetries
	}

	interval := 30 * time.Second
	if conf.Conf.Sync != nil && conf.Conf.Sync.Interval > 0 {
		interval = conf.Conf.Sync.Interval
	}

	deleter := service.NewSocialPlatformDeleter(clients)
	publishWorker := service.NewPublishWorker(postStore, mediaStore, clients, maxRetries, service.WithWorkerDeleter(deleter))

	startWorker(interval, func(ctx context.Context) {
		if err := publishWorker.Run(ctx); err != nil {
			logger.Error("Publish worker failed", "error", err)
		}
	})

	logger.Info("Publish worker started", "interval", interval, "max_retries", maxRetries)
	return nil
}

func runJob(mainSocial string, socials []string) error {
	logger := log.FromContext(context.Background())
	logger.Info("Running job", "main_social", mainSocial, "socials", socials)

	socialService, err := wire.GetSocialService()
	if err != nil {
		return err
	}
	mongoClient := dao.NewMongoClient()
	postDao := dao.NewPostDao(mongoClient)
	locker := dao.NewLocker(dao.NewRedisClient())
	syncService, err := service.NewSyncService(postDao, socialService, locker, mainSocial, socials)
	if err != nil {
		return err
	}

	interval := 30 * time.Second
	if conf.Conf.Sync != nil && conf.Conf.Sync.Interval > 0 {
		interval = conf.Conf.Sync.Interval
	}

	startWorker(interval, func(ctx context.Context) {
		if err := syncService.Sync(ctx); err != nil {
			logger.Error("Sync failed",
				"error", err)
		}
	})

	return nil
}
