# 配置参考

HyperSync 通过 `butterfly.orx.me/core` 加载 YAML 配置文件，反序列化到 `conf.Conf`（`internal/conf/config.go`）。最小可用配置见根 [`README.md`](../README.md)。

## 顶层结构

```yaml
socials:
  <name>:
    # social.PlatformConfig
    ...

store:
  mongo:
    main:
      uri: mongodb://...
  redis:
    locker:
      addr: ...
```

`store.*` 由 core 框架直接消费，应用代码无需感知。`socials` 是 HyperSync 自己的配置。

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
  host: https://bsky.social   # 当前实现未使用，但保留字段
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
    bluesky: { handle: me.bsky.social, password: <app-password> }

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

## 预留字段

`conf.Config` 包含若干尚未投入使用的字段，列在这里以免误用：

| 字段 | 状态 |
| --- | --- |
| `Sync` (SyncConfig) | 未读取，30s 间隔在 `cmd/main.go:88` 硬编码 |
| `Scheduler` (SchedulerConfig) | 未读取，10 分钟间隔在 `cmd/main.go:64` 硬编码 |
| `Webhook` (WebhookConfig) | 未读取 |
| `Memos` (顶层 MemosConfig) | 未读取（实际使用 `socials.<name>.memos`） |
| `Database` | 未读取（Mongo 由 `store.mongo.main` 提供） |

未来可作为扩展点。
