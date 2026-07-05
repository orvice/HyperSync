# 使用指南

从零部署 HyperSync 并完成日常内容发布的完整流程。面向使用者;设计细节见其他文档。

## 部署

HyperSync 由两个镜像组成:后端(Go 服务)和前端(nginx 托管的 React SPA)。依赖 MongoDB 和 Redis。

```yaml
# docker-compose.yaml 示例
services:
  hypersync:
    image: <backend-image>
    volumes:
      - ./local.yaml:/app/config/local.yaml   # 配置文件路径以实际部署为准
    depends_on: [mongo, redis]

  front:
    image: <front-image>          # 由 front/Dockerfile 构建
    environment:
      BACKEND_URL: http://hypersync:8080
    ports:
      - "80:80"

  mongo:
    image: mongo:7
  redis:
    image: redis:7
```

配置文件最小可用示例见根 [README.md](../README.md),完整参考见 [configuration.md](configuration.md)。三个要点:

1. **`auth` 段必填** —— `jwt_secret` 缺失或为空时服务直接拒绝启动。建议 32 位以上随机字符串。
2. **`storage.s3` 建议配置** —— 不配则媒体落在内存里,重启即丢,且 CDN URL 不可访问(仅限本地开发)。
3. 前端镜像通过 `BACKEND_URL` 环境变量反代所有 `/api` 请求到后端,后端本身无需暴露公网。

首次启动时,后端会用 `auth.username` / `auth.password` 在 MongoDB 中 seed 初始用户;之后改密码走前端 Settings 页,**配置文件里的密码不会覆盖已改过的密码**。

## 登录

访问前端地址,用 `auth` 段配置的用户名密码登录。JWT 有效期 24 小时,过期后前端会自动跳回登录页。

## 发布内容

1. **New Post** → 填写正文,选择可见性(visibility)和同步目标(sync targets)。
   - 可见性只有 `public` 和 `unlisted` 会被同步到平台;`private` / `direct` 仅保存在 HyperSync。
   - 同步目标来自后端 `socials` 配置中启用的平台(Mastodon / Bluesky / Threads / Memos)。
2. **保存为草稿**或**直接发布**。草稿不会被同步。
3. 发布后,后台 worker(默认每 30s 一轮)把内容推送到所选平台,附带已上传的媒体。每个平台的同步状态(成功/失败/重试次数)显示在帖子详情页。

### 媒体

创建/编辑页的 **Add Media** 上传图片或视频(单次请求上限 50MB),存入 S3 并生成 CDN 链接,同步时由各平台拉取。

### 编辑已发布的帖子

编辑保存后,已同步的平台会被标记为待更新,worker 对支持编辑的平台(Mastodon / Memos)调用平台的更新接口;不支持编辑的平台(Bluesky / Threads)保持原帖不动。编辑同时会重置重试计数——**同步失败次数耗尽的平台,编辑一次帖子即可让它重新尝试**。

### 删除帖子

删除会级联删除所有已同步平台上的对应帖子(尽力而为:平台侧删除失败只记录日志,本地记录仍会删除)。删除前的确认框会列出受影响的平台。

## 修改密码

Settings 页 → 输入当前密码和新密码(至少 8 位)。注意:已签发的 JWT 在剩余有效期内(最长 24h)仍然有效。

## 常见问题

| 现象 | 原因与处理 |
| --- | --- |
| 服务启动即退出,日志提示 jwt_secret | `auth` 段未配置或 `jwt_secret` 为空,按上文补配置 |
| 帖子一直不同步 | 检查可见性是否为 `public`/`unlisted`、状态是否已发布、对应平台在 `socials` 中是否 `enabled` |
| 某平台同步失败且不再重试 | 默认重试 3 次(`sync.max_retries`)后停止;编辑一次帖子可重置重试 |
| 上传返回 413 | 超过 50MB 上限 |
| 媒体链接打不开 | 未配置 `storage.s3`(内存存储不对外服务),或 `cdn_domain` 不指向该 bucket |
| `/api/token/*`、上传接口返回 401 | 这些接口与 RPC 一样需要 `Authorization: Bearer <JWT>` |

## 旧同步链路

与 Post 管理并行,原有的「Memos → 各平台」拉取式同步照常工作,配置方式不变(`socials.<name>.sync_to`),详见 [sync-flow.md](sync-flow.md)。两条链路互不干扰,数据也分开存储(见 [data-model.md](data-model.md))。
