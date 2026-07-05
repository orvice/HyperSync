# 模块职责

按目录组织。每一节列出关键文件、对外暴露的类型/函数与简要说明。

## `cmd/`

- `cmd/main.go` —— 进程入口。
  - `NewApp()`：用 `core.New` 装配 App。
  - `InitJob()`：遍历 `conf.Conf.Socials`，为每个配置了 `sync_to` 的平台调用 `wire.NewSyncService` 并启动 30s 间隔的同步 goroutine。
  - `InitAuth()`：确保用户索引并按 `auth.username`/`auth.password` 种入管理员账号。
  - `InitPublishWorker()`：启动 `PublishWorker` goroutine（间隔/重试复用 `sync.interval`/`sync.max_retries`）。
  - `InitTokenRefresh()`：构造 `SchedulerService`，启动 10 分钟间隔的 token 刷新调度器。

## `internal/conf/`

- `config.go` —— 顶层 `Config` 结构体与 `Conf` 单例。
  - 包含 `Socials map[string]*social.PlatformConfig`、`Database`、`Memos`、`Sync`、`Scheduler`、`Webhook`、`Auth`、`Storage` 等子配置。
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
| `auth_service.go` | `AuthService` | ConnectRPC `api.v1.AuthService` 实现：`Login`（签发 JWT）/`ChangePassword` |
| `post_service.go` | `PostService` | ConnectRPC `api.v1.PostService` 实现：Post CRUD + `PublishPost`，可选注入 `PlatformDeleter` 做跨平台删除 |
| `media_service.go` | `MediaService` | ConnectRPC `api.v1.MediaService` 实现 + `HandleUpload`（`POST /api/media/upload`） |
| `publish_worker.go` | `PublishWorker` | 后台发布 worker：轮询待发布 Post 并跨发到 `sync_targets`，复用 `sync.interval`/`sync.max_retries` |
| `platform_deleter.go` | `SocialPlatformDeleter` | 将 `social.SocialClient` 集合适配为 `PlatformDeleter` |
| `content_converter.go` | `ContentConverter` | Memo → Post 转换的辅助方法（目前未直接被 SyncService 调用） |

## `internal/auth/`

认证基础设施。

| 文件 | 内容 |
| --- | --- |
| `user.go` | `User` 模型与 `UserStore` 接口 |
| `mongo_store.go` | `MongoUserStore`，`users` 集合 |
| `seed.go` | `SeedUser`：启动时用 `auth.username`/`auth.password` 种入管理员账号 |
| `interceptor.go` | `NewAuthInterceptor`：ConnectRPC 拦截器，除 `/api.v1.AuthService/Login` 外校验 Bearer JWT |

## `internal/post/`

Post 领域层：`store.go` 定义 `Post` 模型与 `Store` 接口，`mongo_store.go`/`memory_store.go` 为 Mongo 与内存实现（内存实现用于测试）。

## `internal/media/`

Media 领域层：`store.go` 定义 `Media` 模型与 `Store` 接口（`mongo_store.go`/`memory_store.go` 实现）；对象存储抽象 `ObjectStorage` 由 `s3.go`（S3 兼容存储，`storage.s3` 配置）或 `memory_object_storage.go`（未配置 S3 时的回退）实现。

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

- `route.go` —— `Router(*gin.Engine)`：注册 `/ping`、`/api/token/*` 路由，并通过 `mountConnectRPC` 挂载 `AuthService`/`PostService`/`MediaService` 三个 ConnectRPC handler（均套用 JWT 拦截器）与 `POST /api/media/upload` 上传端点。

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

- `proto/api/v1/auth.proto` —— `AuthService`（`Login`/`ChangePassword`）。
- `proto/api/v1/post.proto` —— `PostService`（`CreatePost`/`GetPost`/`ListPosts`/`UpdatePost`/`PublishPost`/`DeletePost`）。
- `proto/api/v1/media.proto` —— `MediaService`（`GetMedia`/`ListMedia`/`DeleteMedia`）。
- `pkg/proto/api/v1/` —— `buf generate` 产出的代码：`{auth,post,media}.pb.go` 及 gRPC/Twirp stub；`v1connect/` 下为 Connect stub。
- 重新生成命令：`make buf`。

**注意**：旧的 `proto/api/v1/hyper.proto`（`HyperSyncService`）已删除。Connect handler 在 `internal/http/route.go` 中挂载到 Gin router 对外暴露；gRPC/Twirp stub 仅生成、未挂载。
