# 平台支持

## 能力矩阵

| 平台 | 类型常量 | `ListPosts` | `Post` | 媒体支持 | 鉴权 | Token 自管理 |
| --- | --- | --- | --- | --- | --- | --- |
| Memos | `PlatformMemos` | ✅ | ❌ (未实现) | 读取附件 → Media | Bearer Token | ❌ |
| Mastodon | `PlatformMastodon` | ✅ | ✅ | 多图上传 (`UploadMediaFromBytes`) | Access Token | ❌ |
| Bluesky | `PlatformBluesky` | ✅（502/503 优雅降级） | ✅ | 自动压缩到 976 KB | Handle + App Password | botsky 内部维护会话 |
| Threads | `PlatformThreads` | ❌ (API 未提供) | ✅ (text / image / video / carousel) | 仅支持 URL，不支持 bytes | Client ID/Secret + 长期 Access Token | ✅ 7 天阈值自动刷新 |

## 可见性映射

定义见 `internal/social/social.go:87`：

| 平台 | 支持的 `VisibilityLevel` |
| --- | --- |
| Mastodon | Public, Unlisted, Private, Direct |
| Bluesky | Public, Private |
| Threads | Public, Private |
| Memos | Public, Unlisted, Private |

Memos 的字符串值不同于其他平台：`PUBLIC` / `PROTECTED` / `PRIVATE`。`GetPlatformVisibilityString` 与 `ParsePlatformVisibility` 负责双向转换。

跨发时如果目标平台不支持源帖子的可见性级别，对应平台客户端的 `Post` 会返回错误（如 `"visibility unlisted is not supported by platform bluesky"`），该错误会被记录到 `CrossPostStatus.Error` 中。

## 平台实现细节

### Memos (`internal/social/memos.go`)

- 自研 REST 客户端，base path `/api/v1`。
- 关键端点：`GET /memos`（列表，默认按 `display_time desc` 排序）、`GET /memos/{name}`、`POST /memos`、`PUT /memos/{name}`、`DELETE /memos/{name}`、`GET /users/me`。
- 同时兼容新版 `Attachment` 字段与旧版 `Resource` 字段，URL 拼接为 `{endpoint}/file/{name}/{filename}`。
- `Post` 方法目前返回 `Memos Post method not implemented yet`——Memos 作为**只读源**使用。

### Mastodon (`internal/social/mastodon.go`)

- 基于 `github.com/mattn/go-mastodon`。
- 媒体处理：调用 `Media.GetData()` 拉取字节流，然后 `UploadMediaFromBytes` → 收集 `media_ids` → `PostStatus`。
- `ListPosts` 调用 `GetAccountCurrentUser` + `GetAccountStatuses`。

### Bluesky (`internal/social/bluesky.go`)

- 基于 `github.com/davhofer/botsky`，构造时立即 `Authenticate`，认证失败会导致 `InitSocialPlatforms` 整体失败。
- 媒体处理：所有图片在上传前都会过 `resizeImageIfNeeded`，超过 976 KB 时按比例最近邻缩放，必要时迭代降 JPEG 质量；botsky 需要文件路径，所以会先写到临时文件再删除。
- `ListPosts`：对 502/503 等服务端错误返回空切片，避免阻塞其他平台同步。
- `Post` 返回 `{uri, cid, rkey}`，其中 `rkey` 是从 `at://did/app.bsky.feed.post/rkey` 解析出的最后一段。

### Threads (`internal/social/threads.go`)

发布流程（Meta Graph API 强制）：

1. `CreateMediaContainer` → 拿到 container ID
2. `PublishMediaContainer` → 发布

支持 4 种 `media_type`：`TEXT` / `IMAGE` / `VIDEO` / `CAROUSEL`。`Post(ctx, *Post)` 内部根据 `len(post.Media)` 自动选择类型，并强制要求 `Media.URL` 非空（不支持 bytes 上传）。

**Token 生命周期**（独立于普通发布流程）：

- 短期 token → 长期 token：`ExchangeForLongLivedToken`（`grant_type=th_exchange_token`），需要 `client_secret`。
- 长期 token 刷新：`RefreshLongLivedToken`（`grant_type=th_refresh_token`），无需 secret，刷新后有效期 60 天。
- 约束：长期 token 必须**至少 24 小时旧**才能刷新；剩余有效期 ≤ 7 天时由 `SchedulerService` 自动触发刷新。
- 存储：通过 `TokenManager` 接口（由 `dao.ThreadsConfigAdapter` 实现）写入 `social_configs` 集合，包含 `access_token` 与 `expires_at`。
- 首次启动：`NewThreadsClientWithDao` 优先用 DB 中的 token；DB 为空则把 YAML 里的 `access_token` 写入 DB。
