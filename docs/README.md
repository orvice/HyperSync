# HyperSync 文档

HyperSync 是一个个人内容发布中枢：在 HyperSync 中创作 Post（React 前端 + ConnectRPC API），由后台 worker 同步到 Mastodon、Bluesky、Threads、Memos 等社交平台；同时保留原有的「从 Memos 拉取并跨发」同步链路。本目录是项目的使用与架构参考。

## 文档导航

| 文档 | 内容 |
| --- | --- |
| [usage.md](usage.md) | **使用指南**：部署、登录、发布/编辑/删除、媒体、常见问题 |
| [architecture.md](architecture.md) | 整体架构、运行时拓扑、依赖关系图 |
| [modules.md](modules.md) | 各 Go 包职责与边界 |
| [sync-flow.md](sync-flow.md) | 同步主循环时序、过滤策略、状态流转 |
| [platforms.md](platforms.md) | 平台支持矩阵、各平台客户端实现细节、Token 生命周期 |
| [data-model.md](data-model.md) | MongoDB 集合与文档结构 |
| [api.md](api.md) | HTTP REST 接口与 Proto/Connect 服务定义 |
| [configuration.md](configuration.md) | 配置项完整参考 |

仓库根 [`README.md`](../README.md) 给出最小可用配置与启动方式，前端开发见 [`front/README.md`](../front/README.md)，这里只补充设计层面的细节，不重复其内容。

## 一句话定位

两条链路并行：**发布链路**——在 HyperSync 创作 Post，PublishWorker 按 `sync_pending` 队列推送到所选平台并跟踪每个平台的状态（支持编辑重同步与级联删除）；**同步链路**——读 Memos → 写库去重 → 并行同步到目标平台，由 Redis 分布式锁保护，支持多副本部署。
