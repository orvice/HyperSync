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

## Proto / gRPC / Twirp / Connect

定义：`proto/api/v1/hyper.proto`。

```protobuf
service HyperSyncService {
  rpc ListPosts(ListPostsRequest) returns (ListPostsResponse);
  rpc GetPost(GetPostRequest) returns (GetPostResponse);
  rpc CreatePost(CreatePostRequest) returns (CreatePostResponse);
  rpc DeletePost(DeletePostRequest) returns (DeletePostResponse);
}
```

`buf generate` 会同时产出三种客户端 stub：

| 协议 | 输出包 |
| --- | --- |
| gRPC | `pkg/proto/api/v1/hyper_grpc.pb.go` |
| Twirp | `pkg/proto/api/v1/hyper.twirp.go` |
| Connect | `pkg/proto/api/v1/v1connect/hyper.connect.go` |

**注意**：服务端实现位于 `internal/service/hyper.go`，但 `cmd/main.go` 当前只挂载了 Gin Router，没有显式启动 gRPC/Twirp/Connect server。这部分接口尚未对外暴露。

重新生成命令：

```bash
make buf
```

## Prometheus 指标

`butterfly.orx.me/core` 默认会暴露 `/metrics` 端点。HyperSync 业务指标前缀为 `hyper_sync_*`，定义见 [sync-flow.md](sync-flow.md#span-与指标)。
