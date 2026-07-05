# 数据模型

MongoDB 数据库名硬编码为 `hypersync`（见 `internal/dao/mongo.go:21`）。Redis 仅用于分布式锁，不持久化业务数据。

## 集合一览

| 集合 | 归属 | 用途 |
|---|---|---|
| `managed_posts` | Post 管理（新） | HyperSync 原生创作的 Post，发布 worker 的工作队列 |
| `media` | Post 管理（新） | 上传到 S3 的媒体元数据 |
| `users` | Post 管理（新） | 单用户认证（bcrypt 密码哈希） |
| `posts` | 旧同步链路 | 从源平台拉取的帖子及其跨发状态 |
| `social_configs` | 旧同步链路 | Threads 长期 token |
| `sync_records` | 旧同步链路 | 未启用 |

```mermaid
erDiagram
    POSTS ||--o{ CROSS_POST_STATUS : "embeds map"
    SOCIAL_CONFIGS ||--|| SOCIAL_CONFIG_FIELDS : "embeds"

    POSTS {
        ObjectID _id
        string social "主源平台名"
        string social_id "源 ID"
        string content
        string visibility
        string source_platform
        string original_id
        string[] media_ids
        time created_at
        time updated_at
        map cross_post_status "string→CrossPostStatus"
    }

    CROSS_POST_STATUS {
        bool success
        string error
        string platform_id "目标平台返回 ID"
        bool cross_posted
        time posted_at
        int retry_count "失败重试次数"
    }

    SOCIAL_CONFIGS {
        ObjectID _id
        string platform "平台名（如 threads）"
        int64 user_id
        SocialConfig config
        time created_at
        time updated_at
    }

    SOCIAL_CONFIG_FIELDS {
        string access_token
        time expires_at "Threads 长期 token 过期时间"
    }
```

## `managed_posts` 集合（Post 管理）

Go 模型：`post.postDocument`（`internal/post/mongo_store.go`），Store 接口见 `internal/post/store.go`。

HyperSync 作为内容源头（source of truth）创作的 Post。**刻意与旧链路的 `posts` 集合分开**——`posts` 上有 `(social, social_id)` 非稀疏唯一索引，托管 Post 没有这两个字段，混用会触发 E11000。

关键字段：
- `status`：`draft` | `published`。草稿不会被发布 worker 拾取。
- `sync_pending`：worker 工作队列标记。服务层在创建/发布/编辑已发布 Post 时置 `true`；worker 每轮处理后按「是否仍有未完成目标」重算。`ListPendingSync` 只查 `sync_pending=true`，每轮最多 200 条。
- `cross_post_status[target]`：每目标平台的同步状态（含 `platform_id`、`retry_count`、`needs_update`）。worker 用字段级 `$set`（`UpdateSyncStatus`）写入单个平台的状态，避免整文档覆盖用户并发编辑。
- 编辑已发布 Post 会给已同步平台打 `needs_update` 并把 `retry_count` 归零（给失败平台恢复机会）。

索引（`EnsureIndexes`，`InitPublishWorker` 时创建）：`(sync_pending, status)`、`(status, created_at desc)`。

## `media` 集合（Post 管理）

Go 模型：`media.mediaDocument`（`internal/media/mongo_store.go`）。存 S3 key、CDN URL、content type、大小、原始文件名。二进制内容在 S3（未配置 S3 时退化为内存存储，仅限开发）。

## `users` 集合（Post 管理）

Go 模型：`auth.userDocument`（`internal/auth/mongo_store.go`）。单用户（`auth.username` 配置项在启动时 seed），密码为 bcrypt 哈希。`username` 上有唯一索引。

## `posts` 集合

Go 模型：`dao.PostModel`（`internal/dao/post.go:49`）。

去重键：`(social, social_id)`，通过 `GetBySocialAndSocialID` 查询。启动时 `InitIndexes` 会确保该唯一索引存在（`EnsureIndexes`）。

每条 post 同时记录：
- `source_platform` / `original_id`：源平台的视角（与 `social` / `social_id` 等价，因为 Sync 仅以 main social 作为 source）。
- `cross_post_status[target]`：每个目标平台的最终状态。键集合等于配置中 `sync_to` 的元素。

`PostDao` 接口对外暴露的方法：

```go
GetPostByID(ctx, id) (*PostModel, error)
GetPostByOriginalID(ctx, platform, originalID) (*PostModel, error)
GetBySocialAndSocialID(ctx, social, socialID) (*PostModel, error)
ListPosts(ctx, filter, limit, skip) ([]*PostModel, error)
CreatePost(ctx, *PostModel) (string, error)
UpdatePost(ctx, *PostModel) error
DeletePost(ctx, id) error
UpdateCrossPostStatus(ctx, postID, platform, status) error
```

## `social_configs` 集合

Go 模型：`dao.SocialConfigModel`（`internal/dao/social_config.go:17`）。

目前只用来存放 Threads 的长期 access token 与过期时间。读写通过 `social.TokenManager` 接口，实现是 `dao.ThreadsConfigAdapter`。

主键：`platform`（按平台 upsert）。

注：`SocialConfig.GetThreadsConfig` 在 `social_config.go:44` 引用了 `config.ClientID` 字段，但 `SocialConfig` 结构体本身没有这个字段——这是历史遗留，目前不会触发（`GetThreadsConfig` 没有被生产路径调用）。

## `sync_records` 集合

Go 模型：`dao.SyncRecordModel`（`internal/dao/sync_record.go:18`）。

**当前未被启用的同步路径**所使用。`SyncService` 选用 `posts` + `cross_post_status` 的方案，因此该集合在生产中通常为空。`PostService.SyncPost` / `StartSyncJob` 是另一套实现，使用此集合但目前未由 `cmd/main.go` 调用。

如果将来要清理：可以删除 `sync_record.go` 与 `MongoDAO` 上对应的方法，或保留作为备用。

## Redis

- 用途：分布式锁。
- 客户端来源：`butterfly.orx.me/core/store/redis` 的 `"locker"` 配置项。
- 锁键：
  - `sync_service:<mainSocial>`：每个源平台独立锁，2 分钟 TTL，且有锁续期 watchdog（每 TTL/2 刷新）。
  - `token_refresh`：每个刷新周期 5 分钟 TTL（`scheduler_service.go`）。
