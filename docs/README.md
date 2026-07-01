# HyperSync 文档

HyperSync 是一个多平台内容同步服务：从主源（通常是 Memos）拉取动态，并跨发到 Mastodon、Bluesky、Threads 等社交平台。本目录是项目的架构与功能参考。

## 文档导航

| 文档 | 内容 |
| --- | --- |
| [architecture.md](architecture.md) | 整体架构、运行时拓扑、依赖关系图 |
| [modules.md](modules.md) | 各 Go 包职责与边界 |
| [sync-flow.md](sync-flow.md) | 同步主循环时序、过滤策略、状态流转 |
| [platforms.md](platforms.md) | 平台支持矩阵、各平台客户端实现细节、Token 生命周期 |
| [data-model.md](data-model.md) | MongoDB 集合与文档结构 |
| [api.md](api.md) | HTTP REST 接口与 Proto/Connect 服务定义 |
| [configuration.md](configuration.md) | 配置项完整参考 |

仓库根 [`README.md`](../README.md) 给出最小可用配置与启动方式，这里只补充设计层面的细节，不重复其内容。

## 一句话定位

读 Memos → 写库去重 → 并行同步到目标平台 → 记录每个目标的成功/失败状态以便后续重试，整个过程被 Redis 分布式锁保护，支持多副本部署。
