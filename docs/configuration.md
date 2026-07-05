# 配置参考

HyperSync 通过 `butterfly.orx.me/core` 加载 YAML 配置文件，反序列化到 `conf.Conf`（`internal/conf/config.go`）。最小可用配置见根 [`README.md`](../README.md)。

## 顶层结构

```yaml
socials:
  <name>:
    # social.PlatformConfig
    ...

auth:
  username: admin
  password: <password>
  jwt_secret: <至少 32 位随机字符串>

storage:
  s3:
    # S3 兼容对象存储
    ...

store:
  mongo:
    main:
      uri: mongodb://...
  redis:
    locker:
      addr: ...
```

`store.*` 由 core 框架直接消费，应用代码无需感知。`socials`、`auth`、`storage` 是 HyperSync 自己的配置。

## `socials.<name>` (social.PlatformConfig)

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `name` | string | 平台名（默认取 map key） |
| `type` | string | `memos` / `mastodon` / `bluesky` / `threads` |
| `enabled` | bool | 是否初始化客户端 |
| `sync_enabled` | bool | 是否允许其他平台同步内容**到**这里（与 `sync_from_platforms` 配合） |
| `sync_to` | []string | 将本平台作为主源，同步**到**这些目标平台。**任何 `len(sync_to) > 0` 的平台都会拉起一个独立的同步 goroutine** |
| `sync_from_platforms` | []string | 配合 `sync_enabled`，限制可以同步进来的源平台（`*` 表示任意） |
| `sync_categories` | []string | 预留，未在 SyncService 中使用 |
| `mastodon` | object | Mastodon 子配置 |
| `bluesky` | object | Bluesky 子配置 |
| `memos` | object | Memos 子配置 |
| `threads` | object | Threads 子配置 |

### `mastodon`

```yaml
mastodon:
  instance: https://mastodon.world
  token: <access token>
```

### `bluesky`

```yaml
bluesky:
  host: https://bsky.social   # 启动时校验非空，但 botsky 实际调用不使用
  handle: your-handle.bsky.social
  password: <app password>
```

### `memos`

```yaml
memos:
  endpoint: https://memos.example.com
  token: <bearer token>
```

### `threads`

```yaml
threads:
  client_id: "xxx"
  client_secret: "yyy"      # 用于短期→长期 token 交换
  access_token: "zzz"       # 首次启动时种入数据库，之后以数据库为准
  user_id: 1234567890
  expires_at: 2026-12-31T00:00:00Z   # 可选
```

Threads token 一旦写入 `social_configs` 集合，YAML 中的 `access_token` 即被忽略。

## `auth` 配置（conf.AuthConfig）

```yaml
auth:
  username: admin
  password: <password>
  jwt_secret: <至少 32 位随机字符串>
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `username` | string | 管理员用户名，启动时种入 `users` 集合 |
| `password` | string | 管理员密码（仅首次种入，之后可通过 `ChangePassword` RPC 修改） |
| `jwt_secret` | string | JWT（HMAC）签名密钥，所有 RPC 认证依赖它 |

**`auth.jwt_secret` 为必填项**：缺少 `auth` 配置段或 `jwt_secret` 为空时服务会启动失败。建议使用至少 32 位随机字符作为密钥。

## `storage.s3` 配置（conf.S3Config）

媒体上传（`POST /api/media/upload`）与 `MediaService` 使用的对象存储。未配置时回退到内存存储（仅用于开发，重启即丢失）。

```yaml
storage:
  s3:
    endpoint: https://s3.example.com
    bucket: hypersync
    access_key: <access key>
    secret_key: <secret key>
    region: us-east-1
    cdn_domain: https://cdn.example.com
```

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `endpoint` | string | S3 兼容存储端点 |
| `bucket` | string | Bucket 名 |
| `access_key` | string | Access Key |
| `secret_key` | string | Secret Key |
| `region` | string | 区域 |
| `cdn_domain` | string | 对外访问域名，用于拼接返回的 `cdn_url` |

## 配置示例：单源多目标

```yaml
socials:
  memos:
    name: memos
    type: memos
    enabled: true
    sync_to: [mastodon, bluesky, threads]   # 触发一个 sync goroutine
    memos:
      endpoint: https://memos.example.com
      token: <bearer>

  mastodon:
    name: mastodon
    type: mastodon
    enabled: true
    mastodon: { instance: https://mastodon.world, token: <token> }

  bluesky:
    name: bluesky
    type: bluesky
    enabled: true
    bluesky: { host: https://bsky.social, handle: me.bsky.social, password: <app-password> }

  threads:
    name: threads
    type: threads
    enabled: true
    threads:
      client_id: "..."
      client_secret: "..."
      access_token: "..."
      user_id: 1234567890
```

## Sync 配置

`conf.SyncConfig` 中以下字段已投入使用（默认值在代码中硬编码，配置文件可覆盖）：

| 字段 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `interval` | duration | 30s | 同步轮询间隔（`cmd/main.go`） |
| `batch_size` | int | 100 | 每次拉取帖子数量上限（`sync_service.go`） |
| `skip_older` | duration | 1h | 跳过早于此时长的旧帖（`sync_service.go`） |
| `max_retries` | int | 3 | 跨发失败最大重试次数（`sync_service.go`） |

发布 worker（`PublishWorker`，负责把 `PostService` 创建的帖子跨发到目标平台）复用 `sync.interval` 与 `sync.max_retries`，没有独立的配置项。

以下字段已定义但未被读取：`skip_private`、`max_memos_per_run`、`target_platforms`。

## 预留字段

`conf.Config` 包含若干尚未投入使用的字段，列在这里以免误用：

| 字段 | 状态 |
| --- | --- |
| `Scheduler` (SchedulerConfig) | 未读取，10 分钟间隔在 `cmd/main.go` 硬编码 |
| `Webhook` (WebhookConfig) | 未读取 |
| `Memos` (顶层 MemosConfig) | 未读取（实际使用 `socials.<name>.memos`） |
| `Database` | 未读取（Mongo 由 `store.mongo.main` 提供） |

未来可作为扩展点。
