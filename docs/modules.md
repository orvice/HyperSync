# 模块职责

按目录组织。每一节列出关键文件、对外暴露的类型/函数与简要说明。

## `cmd/`

- `cmd/main.go` —— 进程入口。
  - `NewApp()`：用 `core.New` 装配 App。
  - `InitJob()`：遍历 `conf.Conf.Socials`，为每个配置了 `sync_to` 的平台调用 `wire.NewSyncService` 并启动 30s 间隔的同步 goroutine。
  - `InitTokenRefresh()`：构造 `SchedulerService`，启动 10 分钟间隔的 token 刷新调度器。

## `internal/conf/`

- `config.go` —— 顶层 `Config` 结构体与 `Conf` 单例。
  - 包含 `Socials map[string]*social.PlatformConfig`、`Database`、`Memos`、`Sync`、`Scheduler`、`Webhook` 等子配置。
  - 实际 YAML 反序列化由 `butterfly.orx.me/core` 完成。

注意：`Config.SocialConfig` 与 `social.PlatformConfig` 是两套不同的结构体（前者历史遗留，后者是当前生效的平台配置）。

## `internal/social/`

平台抽象层。

| 文件 | 内容 |
| --- | --- |
| `social.go` | 核心抽象：`Platform` 常量、`VisibilityLevel` 枚举、可见性映射表、`SocialClient`/`TokenManager` 接口、`Post`/`Media` 值对象、`InitSocialPlatforms` 工厂、`CrossPost` 跨发逻辑 |
| `config.go` | `PlatformConfig` 与各平台子配置（`MastodonConfig`/`BlueskyConfig`/`MemosConfig`/`ThreadsConfig`），以及 `ShouldSyncPost` 判断 |
| `memos.go` | Memos REST 客户端（自研，含 Memos v1 API list/get/create/update/delete） |
| `mastodon.go` | Mastodon 客户端，基于 `mattn/go-mastodon` |
| `bluesky.go` | Bluesky 客户端，基于 `davhofer/botsky`，附带图片自动缩放到 976 KB 以下 |
| `threads.go` | Threads Graph API 客户端，包括 token 交换/刷新与 text/image/video/carousel 三步发布流程 |

`SocialClient` 接口只有三个方法：
```go
Post(ctx, *Post) (interface{}, error)
ListPosts(ctx, limit) ([]*Post, error)
Name() string
```
跨发逻辑通过 `Post` 的 `Visibility` 与目标平台的 `SupportedVisibilityLevels` 映射决定是否投递。

## `internal/service/`

业务编排层。

| 文件 | 类型 | 职责 |
| --- | --- | --- |
| `social_service.go` | `SocialService` | 平台注册表；`GetPlatform` / `GetAllPlatforms` / `PostToPlatform` |
| `sync_service.go` | `SyncService` | 核心同步循环，详见 [sync-flow.md](sync-flow.md) |
| `scheduler_service.go` | `SchedulerService` | Token 定时检查/刷新、`TokenStatus` 查询 |
| `post_service.go` | `PostService` | 数据库视角的 Post CRUD 与 `SyncPost`/`StartSyncJob`（备用同步实现，目前 `cmd/main.go` 未启用） |
| `content_converter.go` | `ContentConverter` | Memo → Post 转换的辅助方法（目前未直接被 SyncService 调用） |
| `hyper.go` | `HyperSyncService` | Proto 服务端实现（占位/未挂载到 HTTP） |

## `internal/dao/`

数据访问层，基于 `butterfly.orx.me/core/store/mongo` 与 `store/redis`。

| 文件 | 类型 | 用途 |
| --- | --- | --- |
| `mongo.go` | `MongoDAO` | 共用的 Mongo 客户端持有者，数据库名硬编码为 `hypersync` |
| `post.go` | `PostDao` 接口 + `PostModel` + `CrossPostStatus` | `posts` 集合 |
| `sync_record.go` | `SyncRecordModel` | `sync_records` 集合（备用同步实现使用，当前 `SyncService` 不使用） |
| `social_config.go` | `SocialConfigDao` + `SocialConfigModel` | `social_configs` 集合，存放 Threads access token 与过期时间 |
| `threads_config_adapter.go` | `ThreadsConfigAdapter` | 将 `SocialConfigDao` 适配为 `social.TokenManager` |
| `locker.go` | `redislock.Client` | Redis 分布式锁工厂 |

## `internal/http/`

- `route.go` —— `Router(*gin.Engine)`：注册 `/ping` 与 `/api/token/*` 路由。Token 路由内手动 `wire.NewSchedulerService()` 装配 handler。

## `internal/handler/`

- `token_handler.go` —— `TokenHandler` 处理三个 token 管理接口，详见 [api.md](api.md)。

## `internal/wire/`

Google Wire DI。

- `wire.go`（build tag `wireinject`）：定义 `NewSyncService` / `NewSchedulerService` / `NewMongoDAO` 三个 provider set。
- `wire_gen.go`：`wire` 命令生成的实际装配代码。
- 重新生成命令：`make wire`。

## `internal/metrics/`

- `sync_metrics.go` —— 9 个 Prometheus 指标定义（`hyper_sync_*`，含 `hyper_sync_retries_total`）。
- `helper.go` —— `SyncMetrics` 包装类型，提供 `IncPostsProcessed`/`IncCrossPosts`/`IncErrors`/`TimedOperationWithContext` 等高层 helper。

## `internal/telemetry/`

- `tracing.go` —— `SyncTracer`，封装 OTel `trace.Tracer`，提供语义化的 `StartXxx` / `SetSpanSuccess` / `SetSpanError` / `AddEvent` 方法。

## `proto/` 与 `pkg/proto/`

- `proto/api/v1/hyper.proto` —— 定义 `HyperSyncService`（`ListPosts`/`GetPost`/`CreatePost`/`DeletePost`）。
- `pkg/proto/api/v1/` —— `buf generate` 产出的代码：标准 gRPC、Twirp、Connect 三套 stub 均存在。
- 重新生成命令：`make buf`。

**注意**：当前 `cmd/main.go` 只挂载了 Gin HTTP router，没有显式启动 gRPC/Connect server。Proto 生成代码处于"已就绪但未暴露"状态。
