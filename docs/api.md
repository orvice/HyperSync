# API 参考

## HTTP REST

注册位置：`internal/http/route.go`。所有路由都通过 Gin 暴露。

### `GET /ping`

健康检查。返回 `{"message": "pong"}`。

### `GET /api/token/status/:platform`

查询指定平台的 token 状态。

响应：

```json
{
  "success": true,
  "data": {
    "platform_name": "threads",
    "platform_type": "threads",
    "has_token": true,
    "expires_at": "2026-06-01T10:30:00Z",
    "time_until_expiry": "504h00m00s",
    "is_expiring_soon": false,
    "message": "Token is valid, expires in 504h0m0s"
  }
}
```

非 Threads 平台调用时 `has_token=false` 且 `message="Token management not supported for this platform type"`。

### `POST /api/token/refresh/:platform`

强制刷新指定平台的 token。当前仅支持 `threads`。

成功响应：`{"success": true, "message": "Token refreshed successfully"}`。

错误响应（500）：`{"success": false, "error": "..."}`。

### `POST /api/token/refresh-all`

遍历所有平台并对 Threads 平台执行 `EnsureValidToken`（仅在剩余有效期 ≤ 7 天时实际刷新）。同步执行，无 body。

### `POST /api/media/upload`

媒体上传，`multipart/form-data`，文件字段名为 `file`。需要 `Authorization: Bearer <JWT>` 请求头（token 由 `AuthService/Login` 签发），上传大小限制 50MB。

成功响应：

```json
{
  "id": "665f1a...",
  "cdn_url": "https://cdn.example.com/media/2026/07/03/<uuid>",
  "s3_key": "media/2026/07/03/<uuid>",
  "content_type": "image/png",
  "size_bytes": 12345
}
```

## ConnectRPC 服务

定义：`proto/api/v1/auth.proto`、`proto/api/v1/post.proto`、`proto/api/v1/media.proto`。服务端实现位于 `internal/service/`，在 `internal/http/route.go` 中通过 `v1connect.NewXxxServiceHandler` 挂载到 Gin Router，走 ConnectRPC 协议（同时兼容 gRPC / gRPC-Web / HTTP POST + JSON）。

**破坏性变更**：旧的 `proto/api/v1/hyper.proto` 及其 `HyperSyncService`（Twirp/gRPC，含 `hyper.twirp.go`、`v1connect/hyper.connect.go` 等 stub）已删除，由下列三个服务取代。

### 认证要求

除 `AuthService/Login` 外，**所有 RPC 都要求 `Authorization: Bearer <JWT>` 请求头**。JWT 由 Login 签发，使用 `auth.jwt_secret` 做 HMAC 签名校验（`internal/auth/interceptor.go`）；缺失或非法 token 返回 `unauthenticated`。

### AuthService

| RPC | 路径 | 说明 |
| --- | --- | --- |
| `Login` | `/api.v1.AuthService/Login` | 用户名/密码换取 JWT，返回 `token` 与 `expires_at`。**唯一无需认证的 RPC** |
| `ChangePassword` | `/api.v1.AuthService/ChangePassword` | 修改当前用户密码（`current_password` / `new_password`） |

### PostService

| RPC | 路径 | 说明 |
| --- | --- | --- |
| `CreatePost` | `/api.v1.PostService/CreatePost` | 创建帖子（`content`/`visibility`/`status`/`media_ids`/`sync_targets`） |
| `GetPost` | `/api.v1.PostService/GetPost` | 按 id 查询 |
| `ListPosts` | `/api.v1.PostService/ListPosts` | 分页列表（`page_size`/`page`/`status`），返回 `posts` 与 `total` |
| `UpdatePost` | `/api.v1.PostService/UpdatePost` | 更新帖子 |
| `PublishPost` | `/api.v1.PostService/PublishPost` | 触发发布（交由 PublishWorker 跨发到 `sync_targets`） |
| `DeletePost` | `/api.v1.PostService/DeletePost` | 删除帖子（同时尝试删除各平台上的跨发内容） |

### MediaService

| RPC | 路径 | 说明 |
| --- | --- | --- |
| `GetMedia` | `/api.v1.MediaService/GetMedia` | 按 id 查询媒体元数据 |
| `ListMedia` | `/api.v1.MediaService/ListMedia` | 分页列表（`page_size`/`page`），返回 `items` 与 `total` |
| `DeleteMedia` | `/api.v1.MediaService/DeleteMedia` | 删除媒体（同时删除对象存储中的文件） |

上传媒体使用 HTTP 端点 `POST /api/media/upload`（见上文），不走 RPC。

### 生成代码

`buf generate` 会为每个 proto 产出三种 stub：

| 协议 | 输出包 |
| --- | --- |
| gRPC | `pkg/proto/api/v1/{auth,post,media}_grpc.pb.go` |
| Twirp | `pkg/proto/api/v1/{auth,post,media}.twirp.go` |
| Connect | `pkg/proto/api/v1/v1connect/{auth,post,media}.connect.go` |

实际挂载到 HTTP 的只有 Connect handler。重新生成命令：

```bash
make buf
```

## Prometheus 指标

`butterfly.orx.me/core` 默认会暴露 `/metrics` 端点。HyperSync 业务指标前缀为 `hyper_sync_*`，定义见 [sync-flow.md](sync-flow.md#span-与指标)。
