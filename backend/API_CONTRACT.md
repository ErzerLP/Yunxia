# Yunxia Backend API Contract

> 更新时间：2026-05-07
> 对应实现：当前工作树 `backend/` 实际代码（含全局权限模型 + 统一虚拟目录树 V2 + 审计查询 + RSS/qBittorrent MVP + 通知告警）
> 真相源：`backend/internal/interfaces/http/router.go`、`backend/internal/interfaces/http/handler/*.go`、`backend/internal/application/dto/*.go`、`backend/internal/application/service/*_service.go`

本文档只描述**当前后端实际实现**，用于前后端联调、API client 封装与页面功能核对。

前端接入变更说明固定追加在 `backend/FRONTEND_HANDOFF.md`；新增后端模块/接口需要前端适配时，不新建零散交接文档。

## 0. 前端接入速览

### 0.1 当前后端模块总览

| 模块 | Base | 当前能力 | 前端用途 |
|---|---|---|---|
| 初始化 / 认证 | `/api/v1` | 初始化、登录、刷新 token、登出、当前用户能力 | 登录页、初始化页、全局权限渲染 |
| 系统 | `/api/v1/system/*` | version、stats、config 读写 | 管理后台首页、系统设置页 |
| 用户管理 | `/api/v1/users*` | 用户列表、创建、更新、重置密码、撤销令牌 | 用户管理页 |
| ACL | `/api/v1/acl/rules*` | 对用户授予/拒绝 source 内路径权限 | 权限配置页 |
| 存储源 | `/api/v1/sources*` | local/S3/PikPak 源列表、详情、创建、更新、删除、测试 | 存储源管理、侧边栏导航 |
| 传统文件 | `/api/v1/files*` | 按 `source_id + path` 管理文件 | 兼容旧文件页 |
| 上传 | `/api/v1/upload*` | 初始化、分片、完成、会话、取消 | 文件上传 |
| 回收站 | `/api/v1/trash*` | 列表、恢复、永久删除、清空 | 回收站页 |
| 离线任务 | `/api/v1/tasks*` | 创建、列表、详情、暂停、恢复、取消；普通目标按链接类型走 Aria2/qBittorrent，PikPak 目标优先走 provider 原生离线下载 | 离线任务页 |
| RSS 订阅 | `/api/v1/rss*` | RSS 源、订阅规则、条目、手动刷新、BT/magnet 入队、qBittorrent 健康检查 | RSS 追番页 |
| 通知告警 | `/api/v1/notifications*` | Webhook 通道、通知事件、失败重试 | 设置/通知页、RSS 待处理入口 |
| 分享 | `/api/v1/shares*`、`/s/:token` | 分享管理、公开分享访问 | 分享管理页、公开分享页 |
| 审计 | `/api/v1/audit/logs*` | 审计列表、审计详情 | 审计日志页 |
| 统一虚拟目录 V2 | `/api/v2/fs*` | 基于虚拟路径的文件列表、搜索、写操作、下载 | **新文件管理页推荐优先使用** |
| VFS 标签 | `/api/v1/tags*`、`/api/v2/fs/tags*` | 用户自有标签、VFS 节点标签绑定 / 解绑 / 查询 | 文件标签、筛选与整理 |
| WebDAV | `{webdav_prefix}` 默认 `/dav` | WebDAV 客户端访问已暴露的 local/S3/PikPak 等存储源 | 前端主要展示配置，不直接走 JSON API |

### 0.2 新文件管理页推荐接口

如果是新写前端文件管理页面，推荐优先用 VFS v2，不要让页面直接关心底层 source 类型：

| 页面动作 | 推荐接口 |
|---|---|
| 进入根目录 | `GET /api/v2/fs/list?path=/` |
| 进入目录 | `GET /api/v2/fs/list?path=/local/docs` |
| 手动刷新当前目录 | `POST /api/v2/fs/refresh` |
| 搜索 | `GET /api/v2/fs/search?path=/local&keyword=hello` |
| 新建目录 | `POST /api/v2/fs/mkdir` |
| 重命名 | `POST /api/v2/fs/rename` |
| 移动 | `POST /api/v2/fs/move` |
| 复制 | `POST /api/v2/fs/copy` |
| 删除 | `DELETE /api/v2/fs` |
| 生成下载链接 | `POST /api/v2/fs/access-url` |
| 执行下载 | `GET /api/v2/fs/download?...` |
| 上传初始化 | `POST /api/v1/upload/init`，优先传 `target_virtual_parent_path` |

### 0.3 前端统一请求封装建议

普通 JSON 接口建议统一封装响应包络：

```ts
type ApiEnvelope<T> = {
  success: boolean
  code: string
  message: string
  data: T
  meta: {
    request_id: string
    timestamp: string
  }
  error?: {
    details?: unknown
  }
}

async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const token = localStorage.getItem('access_token')
  const res = await fetch(path, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(options.headers ?? {}),
    },
  })

  const payload = (await res.json()) as ApiEnvelope<T>
  if (!res.ok || !payload.success) {
    throw Object.assign(new Error(payload.message), {
      status: res.status,
      code: payload.code,
      requestId: payload.meta?.request_id,
      details: payload.error?.details,
    })
  }
  return payload.data
}
```

下载接口不要用上面的 JSON 封装；local 会返回文件流，S3/PikPak 会返回 302。最简单的调用方式：

```ts
window.location.href = downloadUrl
```

## 1. 通用约定

### 1.1 Base URL

- 传统数据面 / 管理面：`/api/v1`
- 统一虚拟目录树 V2：`/api/v2`
- WebDAV：由系统配置 `webdav_prefix` 决定，默认 `/dav`

### 1.2 响应包络

除下载文件流、302 跳转与 WebDAV 外，REST 接口统一返回：

```json
{
  "success": true,
  "code": "OK",
  "message": "ok",
  "data": {},
  "meta": {
    "request_id": "uuid",
    "timestamp": "RFC3339"
  }
}
```

错误响应：

```json
{
  "success": false,
  "code": "ERROR_CODE",
  "message": "error message",
  "error": {
    "details": null
  },
  "meta": {
    "request_id": "uuid",
    "timestamp": "RFC3339"
  }
}
```

`httpresp.Empty(...)` 的实际 `data` 是 `{}`，不是 `null`。

### 1.3 认证方式

- 普通接口：`Authorization: Bearer <access_token>`
- 下载短链：`access_token` 查询参数
- WebDAV：Basic Auth（用户名 / 密码）

JSON 请求统一使用：

```http
Content-Type: application/json
Authorization: Bearer <access_token>
```

公开接口不需要 Bearer：

- `GET /api/v1/health`
- `GET /api/v1/setup/status`
- `POST /api/v1/setup/init`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `GET /s/:token`

下载接口虽然路由公开，但仍必须满足以下任一条件：

- 带 `Authorization: Bearer <access_token>`
- 或 query 里带短时 `access_token`

### 1.3.1 token 刷新建议

前端收到以下错误时，可以尝试 refresh：

- `401 AUTH_TOKEN_MISSING`
- `401 AUTH_TOKEN_INVALID`

刷新接口：

```http
POST /api/v1/auth/refresh
Content-Type: application/json

{
  "refresh_token": "<refresh_token>"
}
```

refresh 成功后替换本地 access / refresh token；refresh 失败再跳登录页。

### 1.4 时间 / 分页

- 时间字段：RFC3339
- 当前大部分列表 / 搜索接口使用 `page`、`page_size`
- `tasks` / `shares` 列表当前**不返回 total**

### 1.5 非 JSON 响应

| 接口 | 实际行为 |
|---|---|
| `GET /api/v1/files/download` | local：200 文件流；S3/PikPak：302 到 provider 临时 URL |
| `GET /api/v2/fs/download` | local：200 文件流；S3/PikPak：302 到 provider 临时 URL |
| `GET /s/:token` | 文件：302 到 `/api/v2/fs/download` 短期 access_token 地址；目录：200 metadata JSON |
| `WebDAV` | 标准 WebDAV / XML / 文件流响应，不走 JSON 包络 |

### 1.6 审计与日志约定

- 审计写入当前是 **best-effort**
- 审计落库失败时：
  - 主业务接口**不会**因此改成失败
  - runtime log 会记录 `event=audit.write.failed`
- 当前已覆盖的审计范围：
  - 治理面写操作：setup / system config / users / sources / ACL
  - 数据面写操作：files / upload finish / trash restore / tasks / shares
  - WebDAV 写操作：`PUT` / `MKCOL` / `DELETE` / `COPY` / `MOVE`

## 2. 用户、状态与 capability

### 2.1 用户字段

- `role_key`
  - `super_admin`
  - `admin`
  - `operator`
  - `user`
- `status`
  - `active`
  - `locked`

### 2.2 `/auth/me` 返回 capability 列表

当前内建 capability：

- system
  - `system.stats.read`
  - `system.config.read`
  - `system.config.write`
- user
  - `user.read`
  - `user.create`
  - `user.update`
  - `user.lock`
  - `user.password.reset`
  - `user.tokens.revoke`
  - `user.role.assign`
- acl
  - `acl.read`
  - `acl.manage`
- source
  - `source.read`
  - `source.test`
  - `source.create`
  - `source.update`
  - `source.delete`
  - `source.secret.read`
- audit
  - `audit.read`
  - `audit.read_sensitive`
- notification
  - `notification.read`
  - `notification.manage`
- rss
  - `rss.read`
  - `rss.manage`
- cross-user
  - `task.read_all`
  - `task.manage_all`
  - `share.read_all`
  - `share.manage_all`

### 2.3 当前角色语义

| role_key | 说明 |
|---|---|
| `super_admin` | 拥有全部 capability；初始化首用户固定为该角色；保留 runtime ACL bypass |
| `admin` | 具备治理 capability，但没有 `source.secret.read`；只能管理 `operator/user` |
| `operator` | 只读统计、源读取/测试、跨用户任务治理、RSS 只读、通知事件只读；**当前没有**跨用户分享治理 capability |
| `user` | 无治理 capability；主要依赖 ACL 访问数据面 |

### 2.4 当前关键规则

- 初始化首用户固定创建为 `super_admin`
- 禁止移除最后一个激活的 `super_admin`
- `GET /api/v1/sources?view=navigation` 只要求登录；结果会按 ACL 过滤
- 数据面 ACL：存在显式 ACL 规则的 source 会对普通用户立即生效；无规则且未启用多用户时保留单用户兼容放行
- `view=admin` / source 详情 / source 增删改测：按 capability 控制
- `task` / `share`：owner 默认可管理自己的数据；具备跨用户 capability 的角色可跨用户治理
- `rss`：`rss.read` 可查看授权范围内 RSS 数据；`rss.manage` 可创建/更新/刷新/删除 RSS 源、订阅和手动入队
- S3/PikPak 明文 secret 仅 `source.secret.read` 可见；当前仅 `super_admin` 可见
- 审计查询接口要求 `audit.read`
- `audit.read_sensitive` 当前仅为能力位预留；**现阶段没有额外敏感字段解锁差异**

## 3. 路由总览

### 3.1 初始化与认证（`/api/v1`）

| 方法 | 路径 | 权限 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/setup/status` | 无 | - | 200，`{is_initialized,setup_required,has_super_admin}` |
| POST | `/setup/init` | 无，仅未初始化可调用 | `username,password,email` | 201，`{user,tokens}` |
| POST | `/auth/login` | 无 | `username,password` | 200，`{user,tokens}` |
| POST | `/auth/refresh` | 无 | `refresh_token` | 200，`{tokens}` |
| POST | `/auth/logout` | Bearer | `refresh_token` | 200，`{}` |
| GET | `/auth/me` | Bearer | - | 200，`{user,capabilities[]}` |

补充：

- `POST /auth/refresh` 失败返回 `401 AUTH_REFRESH_TOKEN_INVALID`
- `POST /auth/logout` 需要 Bearer + `refresh_token`

### 3.2 system（`/api/v1`）

| 方法 | 路径 | 权限 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/health` | 无 | - | 200，`{status,service,version}` |
| GET | `/system/version` | 已登录 | - | 200，`{service,version,commit,build_time,go_version,api_version}` |
| GET | `/system/stats` | `system.stats.read` | - | 200，系统聚合统计 |
| GET | `/system/config` | `system.config.read` | - | 200，系统配置 |
| PUT | `/system/config` | `system.config.write` | `site_name,multi_user_enabled,default_source_id,max_upload_size,default_chunk_size,webdav_enabled,webdav_prefix,theme,language,time_zone` | 200，更新后的系统配置 |

补充：

- `system/version` 当前 `api_version` 仍返回字符串 `v1`

### 3.3 users（`/api/v1`）

| 方法 | 路径 | 权限 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/users` | `user.read` | query: `page,page_size,keyword,status` | 200，`{items[]}` |
| POST | `/users` | `user.create` + `user.role.assign` | `username,password,email,role_key` | 201，`{user}` |
| PUT | `/users/:id` | `user.update` + `user.role.assign` + `user.lock` | `email,role_key,status` | 200，`{user}` |
| POST | `/users/:id/reset-password` | `user.password.reset` | `new_password` | 200，`{}` |
| POST | `/users/:id/revoke-tokens` | `user.tokens.revoke` | - | 200，`{id,revoked}` |

补充：

- `admin` 只能创建 / 更新 `operator`、`user`
- 相关错误码包括：`ROLE_ASSIGNMENT_FORBIDDEN`、`LAST_SUPER_ADMIN_FORBIDDEN`

### 3.4 ACL（`/api/v1`）

| 方法 | 路径 | 权限 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/acl/rules` | `acl.read` | query: `source_id,path,vfs_node_id`；`source_id` 或 `vfs_node_id` 至少一个 | 200，`{items[]}` |
| POST | `/acl/rules` | `acl.manage` | `vfs_node_id?` 优先；兼容 `source_id,path`；另含 `subject_type,subject_id,effect,priority,permissions,inherit_to_children` | 201，`{rule}` |
| PUT | `/acl/rules/:id` | `acl.manage` | `vfs_node_id?` 优先；兼容 `source_id?,path`；另含 `subject_type,subject_id,effect,priority,permissions,inherit_to_children` | 200，`{rule}` |
| DELETE | `/acl/rules/:id` | `acl.manage` | - | 200，`{}` |

`permissions` 结构：

```json
{
  "read": true,
  "write": false,
  "delete": false,
  "share": false
}
```

`ACLRuleView` 关键字段：

- `vfs_node_id`：可选。存在时为 node-first 规则，运行时优先按 VFS node 当前身份匹配；`inherit_to_children=true` 时按当前父子关系继承，rename/move 后会重新计算。
- `virtual_path`：创建/更新时保存的 VFS path 快照，用于展示、审计和缺少 node id / metadata reader 时的兼容 fallback。
- `source_id,path`：保留旧 source 内路径创建方式。后端会 best-effort 解析对应 metadata VFS node，成功时内部保存 `vfs_node_id + virtual_path`；解析失败但 path 合法时仍按旧路径快照规则保存。
- 显式绑定 node 的 ACL 在 node rename/move 后继续跟随该 node；仅 path-bound 的旧规则不跟随 rename/move。
- 同一最高优先级命中的规则内 `deny` 优先于 `allow`；不同优先级仍按 `priority desc, id asc` 的既有顺序判定。
- `/api/v2/fs/list` / `/api/v2/fs/search` 在服务端按 ACL 过滤 metadata VFS 节点；普通用户不会收到未授权节点名称或未授权挂载点名称。

### 3.5 sources（`/api/v1`）

| 方法 | 路径 | 权限 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/sources?view=navigation` | 已登录 | query: `view=navigation`（默认） | 200，导航视图源列表 |
| GET | `/sources?view=admin` | `source.read` | query: `view=admin` | 200，管理视图源列表 |
| GET | `/sources/:id` | `source.read` | path: `id` | 200，`{source,config,secret_fields,last_checked_at}` |
| POST | `/sources/test` | `source.test` | `SourceUpsertRequest` | 200，测试结果 |
| POST | `/sources/:id/test` | `source.test` | path: `id` | 200，测试结果 |
| POST | `/sources` | `source.create` | `SourceUpsertRequest` | 201，`{source}` |
| PUT | `/sources/:id` | `source.update` | `SourceUpsertRequest` | 200，`{source}` |
| DELETE | `/sources/:id` | `source.delete` | path: `id` | 200，`{deleted,id}` |

`SourceUpsertRequest` 关键字段：

- 通用：`name,driver_type,is_enabled,is_webdav_exposed,webdav_read_only,mount_path,root_path,sort_order`
- local：`config.base_path`
- s3：`config.endpoint,region,bucket,base_prefix,force_path_style` + `secret_patch.access_key/secret_key`
- pikpak（阶段 E 文件写操作、后端暂存上传导入、GCID 条件浏览器直传、PikPak 目标原生离线下载可用）：`config.root_folder_id,platform,disable_media_link,cache_ttl_seconds,download_strategy,proxy_url` + `secret_patch.username/password/refresh_token/captcha_token/device_id`

创建 local 源示例：

```json
{
  "name": "本地资料",
  "driver_type": "local",
  "is_enabled": true,
  "is_webdav_exposed": false,
  "webdav_read_only": true,
  "mount_path": "/local-data",
  "root_path": "/",
  "sort_order": 0,
  "config": {
    "base_path": "D:/data/yunxia/local-data"
  },
  "secret_patch": {}
}
```

创建 S3 源示例：

```json
{
  "name": "S3 媒体库",
  "driver_type": "s3",
  "is_enabled": true,
  "is_webdav_exposed": false,
  "webdav_read_only": true,
  "mount_path": "/media",
  "root_path": "/",
  "sort_order": 10,
  "config": {
    "endpoint": "https://s3.example.com",
    "region": "us-east-1",
    "bucket": "yunxia-demo",
    "base_prefix": "media",
    "force_path_style": true
  },
  "secret_patch": {
    "access_key": "AKIA...",
    "secret_key": "secret..."
  }
}
```

更新 S3 源时，如果不修改密钥，可以不传对应 `secret_patch` 字段；如果要清空密钥，可以传 `null`。

创建 PikPak 源示例（阶段 E 文件写操作、上传导入、PikPak 目标原生离线下载可用）：

```json
{
  "name": "PikPak 媒体库",
  "driver_type": "pikpak",
  "is_enabled": true,
  "is_webdav_exposed": false,
  "webdav_read_only": true,
  "mount_path": "/pikpak",
  "root_path": "/",
  "sort_order": 20,
  "config": {
    "root_folder_id": "",
    "platform": "web",
    "disable_media_link": true,
    "cache_ttl_seconds": 300,
    "download_strategy": "redirect",
    "proxy_url": ""
  },
  "secret_patch": {
    "username": "user@example.com",
    "password": "pikpak-password",
    "refresh_token": "",
    "captcha_token": "",
    "device_id": ""
  }
}
```

PikPak 字段说明：

- `driver_type` 固定为 `pikpak`
- `root_path` 当前必须为 `/`；远端子目录挂载使用 `config.root_folder_id`，不要把 PikPak 远端路径写入 `root_path`
- `config.root_folder_id` 为空表示 PikPak 账号根目录；填写文件夹 ID 表示把该远端文件夹作为挂载根
- `config.platform` 支持 `web` / `android` / `pc`，默认推荐 `web`
- `config.disable_media_link=true` 时下载使用原始文件链接；`false` 时允许优先使用 provider 媒体链接
- `config.cache_ttl_seconds` 控制 PikPak 路径/id 缓存 TTL；后端会在列表/路径解析时写入缓存，并在上传、离线导入、mkdir、rename、move、copy、delete 等写操作后失效该 source/root 缓存
- `config.download_strategy` 当前仅支持 `redirect`，后端鉴权后由 `/files/download` 或 `/api/v2/fs/download` 302 到 PikPak 临时下载链接
- `config.proxy_url` 为可选 PikPak 专用代理地址，支持 `http://host:port` / `https://host:port`，不允许携带用户名密码、query 或 fragment；为空时使用后端运行环境的标准 `HTTP_PROXY/HTTPS_PROXY` 或 `YUNXIA_PIKPAK_PROXY_URL`
- `secret_patch.username/password/refresh_token/captcha_token/device_id` 均按敏感字段处理；更新时省略字段会保留旧值，传 `null` 会清空该字段
- `GET /sources/:id` 对 PikPak 返回 `secret_fields`；默认不在 `config` 中返回明文 secret，具备 `source.secret.read` capability 时才会返回 `config.username/password/refresh_token/captcha_token/device_id`
- token/captcha/device_id 运行态刷新后会更新当前 source 配置并通过 source repository 持久化写回；服务重启后可继续使用最新 refresh/captcha/device 信息
- PikPak provider 请求遇到 429 或 5xx 临时错误时，后端会执行有限次数退避重试；401/403、账号密码错误、captcha required、region blocked 等用户或部署可修正错误不会重试，最终仍按稳定错误码返回
- PikPak provider 返回区域/网络出口限制（例如大陆网络出口被拒绝）时，接口返回 `451 CLOUD_REGION_BLOCKED`；可通过调整后端网络出口、设置运行环境代理或填写 `config.proxy_url` 解决
- PikPak provider 要求人工验证时，接口返回 `422 CLOUD_CAPTCHA_REQUIRED`；如果 provider 返回验证页面，响应会在 `error.details.verification_url` 中给出可打开的验证地址，管理员完成验证后把得到的 `captcha_token` 作为 `secret_patch.captcha_token` 重新提交或测试
- 当离线任务目标解析到 PikPak source 时，后端会优先调用 PikPak 原生离线下载任务，而不是先下载到 Yunxia staging；该优化不改变前端创建任务接口
- PikPak 上传现在同时支持两条后端路径：前端在 `/upload/init.file_hash` 传 `gcid:<40位hex>` 或 `<40位hex>` 时，后端优先创建 provider OSS 直传计划；未传 GCID 或传普通 MD5/空值时，后端自动回退为 `server_chunk -> ImportFile`

补充：

- 初始化完成后自动创建默认本地源：`本地存储`
- 默认本地源当前挂载到 `mount_path=/local`
- `mount_path` 需要全局唯一，冲突返回 `409 MOUNT_PATH_CONFLICT`
- 创建 / 更新 / 删除存储源时，后端会同步维护 metadata VFS 挂载控制面；若同步失败，接口返回 `500 METADATA_VFS_MOUNT_SYNC_FAILED`，不会把 source 操作伪装为成功
- local 源的物理宿主路径必须放在 `config.base_path`；该路径必须已存在且是目录，后端不会为用户创建的 local 源自动创建 `base_path`
- `root_path` 是源内逻辑根路径，不用于承载物理磁盘路径
- local 源缺少 `config.base_path`、`base_path` 不存在 / 不是目录，或路径字段非法时返回 `400 PATH_INVALID`，不返回 500
- `webdav_slug` 由后端按源名称生成并自动去重；前端创建源时无需传入
- `PUT /sources/:id` 当前会保留原有 `driver_type`，不是切换驱动接口
- `GET /sources/:id` 对 S3/PikPak 源返回 `secret_fields`；只有具备 `source.secret.read` 的账号可看到对应 secret 明文

### 3.6 files（`/api/v1`）

| 方法 | 路径 | 鉴权 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/files` | Bearer | query: `source_id,path,page,page_size,sort_by,sort_order` | 200，`{items,current_path,current_source_id}` |
| GET | `/files/search` | Bearer | query: `source_id,keyword,path_prefix,page,page_size` | 200，`{items,keyword,current_source_id,path_prefix}` |
| POST | `/files/mkdir` | Bearer | `source_id,parent_path,name` | 200，`{created}` |
| POST | `/files/rename` | Bearer | `source_id,path,new_name` | 200，`{old_path,new_path,file}` |
| POST | `/files/move` | Bearer | `source_id,path,target_path` | 200，`{old_path,new_path,moved}` |
| POST | `/files/copy` | Bearer | `source_id,path,target_path` | 200，`{source_path,new_path,copied}` |
| DELETE | `/files` | Bearer | `source_id,path,delete_mode` | 200，`{deleted,delete_mode,path,deleted_at}` |
| POST | `/files/access-url` | Bearer | `source_id,path,purpose,disposition,expires_in` | 200，`{url,method,expires_at}` |
| GET | `/files/download` | Bearer 或 `access_token` | query: `source_id,path,disposition[,access_token]` | local：200 文件流；S3/PikPak：302 |

补充：

- `delete_mode` 为空时默认按 `trash`
- 数据面接口会做 ACL 校验；失败返回 `403 ACL_DENIED`
- 本地源 / 挂载目录探测为不可写时，`mkdir` / `rename` / `move` / `copy` / `delete` 返回 `403 SOURCE_READ_ONLY`；响应只暴露稳定错误码与通用消息，不返回容器或宿主机物理路径
- `files/access-url` 对 local / S3 / PikPak 当前都先返回应用内短链 `/api/v1/files/download?...&access_token=...`
- 真正的 S3 presigned URL / PikPak 临时下载链接在 `GET /files/download` 时再 302 跳转
- PikPak 阶段 E 支持 `list` / `search`（受限递归）/ `stat` / `access-url` / `download` / `mkdir` / `rename` / `move` / `copy` / `delete`，并支持后端 staging 文件导入与 PikPak 目标原生离线下载；文件/VFS 条目 `can_delete` 会按 driver capability 与 ACL 共同计算。
- PikPak `delete_mode` 为空或 `trash` 时调用 provider `batchTrash`，语义是移入 PikPak 回收站，不会创建 Yunxia `.trash` 记录；`delete_mode=permanent` 暂返回 `422 SOURCE_OPERATION_UNSUPPORTED`。

### 3.7 upload（`/api/v1`）

| 方法 | 路径 | 鉴权 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| POST | `/upload/init` | Bearer | 两种模式，见下方 | 200，`UploadInitResponse` |
| PUT | `/upload/chunk` | Bearer | query: `upload_id,index`，body 为二进制分片 | 200，`{upload_id,index,received_bytes,already_uploaded}` |
| POST | `/upload/finish` | Bearer | `upload_id[,parts[]]` | 201，`{completed:true,upload_id,file,result_vfs_node_id}` |
| GET | `/upload/sessions` | Bearer | query: `source_id,status` | 200，`{items[]}` |
| DELETE | `/upload/sessions/:upload_id` | Bearer | path: `upload_id` | 200，`{upload_id,canceled}` |

`POST /upload/init` 当前支持两种入参模式：

1. 传统模式：

```json
{
  "source_id": 1,
  "path": "/docs",
  "filename": "hello.txt",
  "file_size": 11,
  "file_hash": "...",
  "last_modified_at": "2026-04-23T12:00:00+08:00"
}
```

2. 统一虚拟目录模式：

```json
{
  "target_virtual_parent_path": "/docs",
  "filename": "hello.txt",
  "file_size": 11,
  "file_hash": "...",
  "last_modified_at": "2026-04-23T12:00:00+08:00"
}
```

补充：

- 若 `target_virtual_parent_path` 非空，会**优先**走虚拟目录解析
- 上传会话 / 初始化响应当前会带：
  - `target_vfs_parent_node_id`：目标父目录对应的 metadata VFS node id；若父目录尚未懒索引入库可能省略
  - `target_virtual_parent_path`
  - `resolved_source_id`
  - `resolved_inner_parent_path`
- 上传 / fast-upload 的 `completed` 语义以 metadata VFS file node + storage object 提交成功为准：成功响应必须带 `result_vfs_node_id`，且随后 `GET /api/v2/fs/list?path=<target_parent>` 立即能看到该 result node
- 若底层文件/对象已写入但 metadata commit 失败，`/upload/finish` 不会返回 `completed=true`，会返回稳定 `500 METADATA_VFS_COMMIT_FAILED` 与安全摘要 `metadata vfs commit failed`；后端会保留可追踪 operation journal，上传 session 不写入 completed/result
- 本地源返回 `transport.mode=server_chunk`
- S3 源返回 multipart 直传说明 `part_instructions[]`
- PikPak 上传会按 `file_hash` 自动分流：
  - `file_hash="gcid:<40位hex>"` 或 `<40位hex>`：优先返回 `transport.mode="direct_parts"` 和一个 PikPak OSS `PUT` 直传说明
  - 未传 GCID、传普通 MD5 或空值：回退为 `transport.mode="server_chunk"`，由后端接收分片合并成本地 staging 文件，再调用 PikPak `ImportFile`
  - provider 秒传时 `/upload/init` 直接返回 `is_fast_upload=true`
- 纯虚拟目录无落地存储时返回 `409 NO_BACKING_STORAGE`

本地源上传调用顺序：

1. `POST /api/v1/upload/init`
2. 按 `upload.chunk_size` 切片
3. 对每片调用 `PUT /api/v1/upload/chunk?upload_id=<id>&index=<0-based>`
   - Header：`Content-Type: application/octet-stream`
   - Body：当前分片二进制
4. `POST /api/v1/upload/finish`

本地源 `PUT /upload/chunk` 响应：

```json
{
  "upload_id": "upl_xxx",
  "index": 0,
  "received_bytes": 5242880,
  "already_uploaded": false
}
```

S3 源上传调用顺序：

1. `POST /api/v1/upload/init`
2. 前端根据 `part_instructions[]` 直接 PUT 到 S3 presigned URL
3. 收集每个分片返回的 ETag
4. `POST /api/v1/upload/finish`，Body 里传 `parts`

S3 finish Body 示例：

```json
{
  "upload_id": "upl_xxx",
  "parts": [
    { "index": 0, "etag": "\"etag-part-1\"" },
    { "index": 1, "etag": "\"etag-part-2\"" }
  ]
}
```

PikPak 上传推荐调用顺序：

1. `POST /api/v1/upload/init`
   - 如果前端能计算 PikPak GCID，可把 `file_hash` 传为 `gcid:<40位hex>` 或 `<40位hex>`
   - 如果前端暂不实现 GCID，继续传普通 MD5 或空值即可，后端会走 `server_chunk`
2. 根据 `transport.mode` 分支：
   - `server_chunk`：
     1. `transport.driver_type="pikpak"`
     2. `part_instructions=[]`
     3. 前端按 `upload.chunk_size` 调 `PUT /api/v1/upload/chunk`
     4. `POST /api/v1/upload/finish`
        - 后端合并 staging 文件后导入 PikPak
        - PikPak 秒传成功时不触发 OSS 实体上传
        - 需要实体上传时由后端使用 provider 返回的 OSS 临时参数上传，前端不接触 `access_key_secret`、`security_token`、`bucket/key`
   - `direct_parts`：
     1. 当前 PikPak OSS 直传返回 1 条 `part_instructions[0]`
     2. 前端按该 instruction 的 `method="PUT"`、`url`、`headers` 与 `byte_range` 直接上传整个文件
     3. 上传成功后调用 `POST /api/v1/upload/finish`，Body 传入 OSS 返回的 ETag，例如：

```json
{
  "upload_id": "upl_xxx",
  "parts": [
    { "index": 0, "etag": "\"etag-from-oss\"" }
  ]
}
```

PikPak `direct_parts` 响应示例：

```json
{
  "is_fast_upload": false,
  "upload": {
    "upload_id": "upl_xxx",
    "source_id": 3,
    "path": "/Anime",
    "filename": "episode.mkv",
    "file_size": 734003200,
    "file_hash": "gcid:0123456789abcdef0123456789abcdef01234567",
    "chunk_size": 5242880,
    "total_chunks": 1,
    "uploaded_chunks": [],
    "status": "uploading",
    "is_fast_upload": false,
    "expires_at": "2026-05-12T08:00:00Z"
  },
  "transport": {
    "mode": "direct_parts",
    "driver_type": "pikpak",
    "concurrency": 3,
    "retry_limit": 3
  },
  "part_instructions": [
    {
      "index": 0,
      "method": "PUT",
      "url": "https://<bucket>.<endpoint>/<key>",
      "headers": {
        "Authorization": "OSS <access_key_id>:<signature>",
        "Date": "Tue, 05 May 2026 08:00:00 GMT",
        "Content-Type": "video/x-matroska",
        "X-OSS-Security-Token": "<temporary-token>"
      },
      "byte_range": { "start": 0, "end": 734003199 },
      "expires_at": "2026-05-05T08:15:00Z"
    }
  ]
}
```

注意：

- PikPak `direct_parts` 返回的是短期 OSS PUT 上传凭据和签名 header，只用于当前文件上传；前端不要持久化、日志输出或复用其中的 `Authorization` / `X-OSS-Security-Token`。
- PikPak direct 当前是单对象 PUT，不是 S3 multipart；`upload.total_chunks` 会等于 `part_instructions.length`，通常为 1。
- PikPak 上传目标父目录不存在时，后端会按目标路径递归创建远端父目录；如果某段父路径已存在但不是目录，返回 `400 PATH_INVALID`。
- 如果 provider 在 init 阶段秒传成功，响应为 `is_fast_upload=true` 且不返回 `upload/transport/part_instructions`。

### 3.8 trash（`/api/v1`）

| 方法 | 路径 | 鉴权 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/trash` | Bearer | query: `source_id,page,page_size` | 200，`{items[]}` |
| POST | `/trash/:id/restore` | Bearer | path: `id` | 200，`{id,restored,restored_path[,restored_virtual_path]}` |
| DELETE | `/trash/:id` | Bearer | path: `id` | 200，`{id,deleted}` |
| DELETE | `/trash` | Bearer | query: `source_id` | 200，`{source_id,cleared,deleted_count}` |

补充：

- `TrashItemView` 当前还会返回 `original_virtual_path`
- 恢复冲突返回 `409 TRASH_RESTORE_CONFLICT`

### 3.9 tasks（`/api/v1`）

| 方法 | 路径 | 鉴权 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/tasks` | Bearer | - | 200，`{items[]}` |
| POST | `/tasks` | Bearer | `type,url,target_virtual_parent_path,target_filename?`；兼容 `type,url,source_id,save_path` | 202，`{task}` |
| GET | `/tasks/:id` | Bearer | path: `id` | 200，**直接返回 `DownloadTaskView`** |
| POST | `/tasks/:id/pause` | Bearer | path: `id` | 200，`{id,status}` |
| POST | `/tasks/:id/resume` | Bearer | path: `id` | 200，`{id,status}` |
| DELETE | `/tasks/:id` | Bearer | query: `delete_file` | 200，`{id,canceled,delete_file}` |

补充：

- 推荐新建任务时传 `target_virtual_parent_path`，语义是“统一虚拟目录中的目标父目录”，后端会解析到具体挂载存储源
- 兼容旧模式：不传 `target_virtual_parent_path` 时，继续使用 `source_id + save_path`
- 旧模式下 `save_path` 是 **source 内部目标父目录**，不是统一虚拟目录路径
- 下载器按目标与链接类型分发：
  - 目标解析到 PikPak source 时，普通 HTTP/HTTPS、`magnet:?` 与 `.torrent` URL 优先走 PikPak provider 原生离线下载，`DownloadTaskView.downloader_type="pikpak_native"`。
  - 其他目标：普通 HTTP/HTTPS 走 Aria2；`magnet:?` 与 `.torrent` URL 走 qBittorrent（未启用 qBittorrent 时返回 `503 DOWNLOADER_UNAVAILABLE`）。
- `target_filename` 为可选任务级目标文件名快照，主要由 RSS `filename_template` 自动生成；直接创建任务也可传入。它只能是文件名，不能包含路径分隔符、`..` 或 Windows drive-style 前缀；非法时返回 `422 FILE_NAME_INVALID`。
- 除 PikPak 原生离线下载外，下载器只写入后端本地 staging 目录；任务完成后由后端导入目标存储源：
  - local 目标：从 staging move/copy 到真实物理路径
  - S3 目标：从 staging 上传到对应对象 key
  - PikPak fallback 目标：从 staging 调 PikPak `ImportFile` 导入；当 PikPak 原生离线下载不可用或未注册时仍保留该路径
  - staging 本地物理路径不会返回给前端
  - 导入 + metadata VFS commit 都成功后才清理 staging 并进入 `completed`；后续刷新如果遇到已导入文件但 staging 已清理，会先确认/补交 metadata result node，不能确认时转为 `failed`
  - 当 `target_filename` 非空且 staging 中只有一个有效文件时，导入阶段会把该文件落到目标父目录下的 `target_filename`；若 `target_filename` 没有明确扩展名（如 `.mkv` / `.mp4`；`S01.05` 这类集数后缀不算扩展名），会保留原下载文件扩展名；多文件任务保持原相对路径，不应用重命名
- PikPak 原生离线下载任务：
  - 创建任务时后端会把目标 VFS 目录解析为 PikPak source 内目录；目标父目录不存在时会由 PikPak driver 递归创建。
  - `target_filename` 非空时会传给 provider 作为任务名，并在创建前检查同目录同名冲突；为空时由 provider 按链接自行命名。
  - 任务完成后文件已经在 PikPak source 中，不再调用 staging 导入；后端仍会提交 metadata VFS result node，`completed` 响应后刷新/列出 VFS 目录即可看到结果。
  - 若 provider reported completed 但既没有 `target_filename` 也没有返回安全文件名，后端会把任务置为 `failed` + `METADATA_VFS_COMMIT_FAILED`，不会用 magnet/URL 字符串伪造 result node 或 `completed`。
  - provider `PHASE_TYPE_PENDING/RUNNING/COMPLETE/ERROR` 分别映射为 `pending/running/completed/failed`。
  - 暂停/恢复当前返回 `422 SOURCE_OPERATION_UNSUPPORTED`；取消会调用 provider 删除任务记录，并透传 `delete_file` 为 provider `delete_files`。
- 返回体当前会补充 VFS 快照字段：
  - `target_vfs_parent_node_id`
  - `target_virtual_parent_path`
  - `target_filename`
  - `save_virtual_path`
  - `resolved_source_id`
  - `resolved_inner_save_path`
  - `result_vfs_node_id`：任务进入 `completed` 时必须返回，指向本次任务提交出的 metadata VFS result file node；多文件任务当前指向第一个成功提交的结果文件，后续如引入目录级 result node 再扩展
- 普通用户默认仅能看到 / 操作自己的任务
- `DELETE /tasks/:id` 对终态任务是幂等的：已 `completed` / `failed` / `canceled` 的任务不会再调用底层下载器取消，因此不会把 Aria2/qBittorrent 的底层 400 暴露给前端；已完成任务保持 `completed`
- 用户主动取消的非终态任务会先在 Yunxia 内记录为 `canceled` 和 `error_message="download canceled by user"`，后续下载器状态刷新不会覆盖该取消原因
- 具备 `task.read_all` / `task.manage_all` capability 的角色可跨用户治理
- 终态任务（`completed` / `failed` / `canceled`）返回时会清空实时下载字段：`speed_bytes=0`、`eta_seconds=null`
- `completed` 任务返回时 `error_message` 固定为 `null`；若导入或 metadata commit 失败，任务会转为 `failed`，`speed_bytes=0`、`eta_seconds=null`，`error_message` 为安全摘要（metadata commit 失败固定 `metadata vfs commit failed`），并记录 `task_commit` operation journal
- `failed` / `canceled` 任务会返回明确 `error_message`；下载器未返回原因时后端补默认原因，用户主动取消时为 `download canceled by user`
- `DownloadTaskView.downloader_type` 当前可能为 `aria2`、`qbittorrent` 或 `pikpak_native`
- ACL / 权限失败统一返回 `403 PERMISSION_DENIED`
- 当前没有 `retry` 接口


### 3.10 rss（`/api/v1/rss`）

RSS MVP 由 Yunxia 管理 RSS 源、订阅规则、条目去重与目标 VFS 目录；qBittorrent 只作为 BT/magnet 下载执行器。

| 方法 | 路径 | 权限 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/rss/export` | `rss.manage` | - | 200，`RSSExportResponse` |
| POST | `/rss/import` | `rss.manage` | `dry_run,sources[],subscriptions[]` | 200，`RSSImportResponse` |
| GET | `/rss/sources` | `rss.read` | - | 200，`{items[]}` |
| POST | `/rss/sources` | `rss.manage` | `name,url,is_enabled,refresh_interval_seconds` | 201，`{source}` |
| GET | `/rss/sources/:id` | `rss.read` | path: `id` | 200，直接返回 `RSSSourceView` |
| PATCH | `/rss/sources/:id` | `rss.manage` | 同创建 | 200，`{source}` |
| DELETE | `/rss/sources/:id` | `rss.manage` | path: `id` | 200，`{deleted,id}` |
| POST | `/rss/sources/refresh-all` | `rss.manage` | - | 200，`RSSRefreshAllResponse` |
| POST | `/rss/sources/:id/refresh` | `rss.manage` | path: `id` | 200，`RSSRefreshResponse` |
| GET | `/rss/subscriptions` | `rss.read` | query: `source_id` | 200，`{items[]}` |
| POST | `/rss/subscriptions` | `rss.manage` | 见下方请求体 | 201，`{subscription}` |
| POST | `/rss/subscriptions/preview` | `rss.manage` | `source_id,must_contain,must_not_contain,use_regex,case_sensitive` | 200，`RSSSubscriptionPreviewResponse`，`subscription_id=0` |
| POST | `/rss/subscriptions/batch-state` | `rss.manage` | `subscription_ids[],is_enabled` | 200，`RSSSubscriptionBatchStateResponse` |
| GET | `/rss/subscriptions/:id` | `rss.read` | path: `id` | 200，直接返回 `RSSSubscriptionView` |
| PATCH | `/rss/subscriptions/:id` | `rss.manage` | 同创建 | 200，`{subscription}` |
| DELETE | `/rss/subscriptions/:id` | `rss.manage` | path: `id` | 200，`{deleted,id}` |
| POST | `/rss/subscriptions/:id/clone` | `rss.manage` | path: `id`，可选 `name,is_enabled` | 201，`{subscription}` |
| POST | `/rss/subscriptions/:id/run` | `rss.manage` | path: `id` | 200，`RSSRefreshResponse` |
| POST | `/rss/subscriptions/:id/preview` | `rss.manage` | path: `id` | 200，`RSSSubscriptionPreviewResponse` |
| GET | `/rss/items` | `rss.read` | query: `source_id,subscription_id,status` | 200，`{items[]}` |
| POST | `/rss/items/batch-ignore` | `rss.manage` | `item_ids[]` | 200，`RSSItemBatchActionResponse` |
| POST | `/rss/items/batch-retry` | `rss.manage` | `item_ids[]`，`subscription_id` 可选 | 202，`RSSItemBatchActionResponse` |
| POST | `/rss/items/:id/download` | `rss.manage` | `subscription_id` 可选 | 202，`{item}` |
| POST | `/rss/items/:id/reprocess` | `rss.manage` | path: `id` | 202，`{item}` |
| POST | `/rss/items/:id/retry` | `rss.manage` | `subscription_id` 可选 | 202，`{item}` |
| GET | `/rss/qbittorrent/health` | `rss.read` | - | 200，`{enabled,status,error}` |

qBittorrent 健康响应语义：

- `enabled=false,status=disabled`：后端未启用 qBittorrent 下载器。
- `enabled=true,status=ok`：后端已连通 qBittorrent Web API。
- `enabled=true,status=unavailable,error=...`：后端已启用但 Web API 不可用；`error` 会保留可诊断的下游状态，例如 `qbittorrent login status 401` 或 `qbittorrent health status 401`。
- 项目 `docker-compose.backend.yml` 内置 sidecar 默认使用内部网络 + qBittorrent WebUI 子网白名单，后端默认 qBittorrent 账号密码为空并跳过登录；sidecar entrypoint 会在每次启动时修正既有配置卷中的 WebUI 白名单/HostHeader/CSRF/SecureCookie 设置。仅改接需要认证的外部 qBittorrent 时设置 `YUNXIA_QBITTORRENT_USERNAME` / `YUNXIA_QBITTORRENT_PASSWORD`。

订阅创建/更新请求体：

```json
{
  "source_id": 1,
  "name": "Frieren 1080p",
  "is_enabled": true,
  "must_contain": ["Frieren", "1080p"],
  "must_not_contain": ["CHT"],
  "use_regex": false,
  "case_sensitive": false,
  "target_virtual_parent_path": "/anime",
  "directory_template": "{anime_title}/{season}",
  "filename_template": "{anime_title} - {episode} [{resolution}]"
}
```

订阅复制请求体可为空；传入字段时只覆盖新订阅的名称和启用状态：

```json
{
  "name": "Frieren 1080p Copy",
  "is_enabled": false
}
```

批量启用/禁用订阅请求体：

```json
{
  "subscription_ids": [1, 2, 3],
  "is_enabled": false
}
```

临时规则 preview 请求体（不落库，不要求 `target_virtual_parent_path`）：

```json
{
  "source_id": 1,
  "must_contain": ["Frieren", "1080p"],
  "must_not_contain": ["CHT"],
  "use_regex": false,
  "case_sensitive": false
}
```

RSS 导出响应 `data` 为 `RSSExportResponse`，只包含可迁移配置，不包含 items、tasks、刷新健康/重试等运行时状态：

```json
{
  "version": 1,
  "exported_at": "2026-05-02T13:00:00+08:00",
  "sources": [
    {
      "name": "Mikan",
      "url": "https://mikan.example/rss.xml",
      "is_enabled": true,
      "refresh_interval_seconds": 1800
    }
  ],
  "subscriptions": [
    {
      "source_url": "https://mikan.example/rss.xml",
      "name": "Frieren 1080p",
      "is_enabled": true,
      "must_contain": ["Frieren", "1080p"],
      "must_not_contain": ["CHT"],
      "use_regex": false,
      "case_sensitive": false,
      "target_virtual_parent_path": "/anime",
      "directory_template": "{anime_title}/{season}",
      "filename_template": "{anime_title} - {episode} [{resolution}]"
    }
  ]
}
```

RSS 导入请求可直接使用导出结构追加 `dry_run`；`subscriptions[].source_ref` 兼容 `source_url` 别名，推荐新前端只写 `source_url`：

```json
{
  "dry_run": true,
  "sources": [
    {
      "name": "Mikan",
      "url": "https://mikan.example/rss.xml",
      "is_enabled": true,
      "refresh_interval_seconds": 1800
    }
  ],
  "subscriptions": [
    {
      "source_url": "https://mikan.example/rss.xml",
      "name": "Frieren 1080p",
      "is_enabled": true,
      "must_contain": ["Frieren", "1080p"],
      "must_not_contain": ["CHT"],
      "use_regex": false,
      "case_sensitive": false,
      "target_virtual_parent_path": "/anime",
      "directory_template": "{anime_title}/{season}",
      "filename_template": "{anime_title} - {episode} [{resolution}]"
    }
  ]
}
```

RSS 导入响应会逐项返回结果；单项失败不导致整体 HTTP 失败，只有 JSON 格式/绑定错误才返回 4xx：

```json
{
  "dry_run": true,
  "sources": {
    "items": [
      {
        "index": 0,
        "action": "reuse",
        "success": true,
        "id": 1,
        "source_url": "https://mikan.example/rss.xml",
        "name": "Mikan"
      }
    ],
    "created": 0,
    "reused": 1,
    "skipped": 0,
    "failed": 0
  },
  "subscriptions": {
    "items": [
      {
        "index": 0,
        "action": "failed",
        "success": false,
        "source_url": "https://mikan.example/rss.xml",
        "name": "Frieren 1080p",
        "error_code": "PATH_INVALID",
        "error_message": "path invalid"
      }
    ],
    "created": 0,
    "reused": 0,
    "skipped": 0,
    "failed": 1
  }
}
```

批量订阅启停响应形状：

```json
{
  "items": [
    {
      "subscription_id": 1,
      "success": true,
      "subscription": { "...": "RSSSubscriptionView" }
    },
    {
      "subscription_id": 2,
      "success": false,
      "error_code": "PERMISSION_DENIED",
      "error_message": "permission denied"
    }
  ],
  "succeeded": 1,
  "failed": 1
}
```

批量条目动作响应统一形状：

```json
{
  "items": [
    {
      "item_id": 10,
      "success": true,
      "item": { "...": "RSSItemView" }
    },
    {
      "item_id": 11,
      "success": false,
      "error_code": "TASK_INVALID_STATE",
      "error_message": "task invalid state"
    }
  ],
  "succeeded": 1,
  "failed": 1
}
```

约束：

- RSS 导出只导出配置字段：source 的 `name/url/is_enabled/refresh_interval_seconds`，subscription 的 `source_url/name/is_enabled/must_contain/must_not_contain/use_regex/case_sensitive/target_virtual_parent_path/directory_template/filename_template`；不会导出 item、task、refresh health、retry 等运行时状态。
- RSS 导出范围沿用 service 授权语义：普通身份只能看到自己的 RSS 数据；具备 `rss.manage` 的身份会按现有 `IncludeAll` 语义导出可管理范围内的全部 RSS 源/订阅。
- RSS 导入中 source 按“当前导入 owner + URL 精确匹配”复用；已存在的 source 不会被覆盖。不存在时创建；`dry_run=true` 只返回将要执行的 `create/reuse/failed` 结果，不落库。
- RSS 导入创建 subscription 时使用 `source_url` 映射导入/已有 source，并重新校验 `target_virtual_parent_path` 可写；目标路径、正则或模板非法只让该 subscription 单项失败。
- 第一版只自动入队 `magnet:?` 和 `.torrent` URL；普通 HTTP/直链条目标记为 `unsupported`，不创建 RSS 下载任务。
- 下载链接解析会检查 RSS item `link`、`enclosure.url` 和 Mikan 扩展 `torrent/link`；只要其中存在 `magnet:?` 或 `.torrent` URL 即可入队。
- RSS 时间解析顺序：RSS 顶层 `pubDate`、Mikan torrent 扩展 `torrent/pubDate`、`date`、Atom `published/updated`；仍无法识别时 `published_at=null`。
- RSS 条目会持久化轻量番剧标题解析结果，并在 `RSSItemView.parsed` 返回：
  - `anime_title`
  - `season`
  - `episode`
  - `subtitle_group`
  - `resolution`
- 非正则关键词匹配：
  - 普通文本关键词会匹配标题与链接元数据。
  - 1~2 位纯数字关键词按“集数”语义处理，只匹配标题中的集数 token / `SxxEyy` / `EPyy` / `第 yy 集`，不会匹配 URL、hash、发布时间或 `1080p` 等元信息。
- `.torrent` URL 入队时后端会先下载 torrent 文件，再以 multipart 文件方式提交给 qBittorrent；避免 qBittorrent 异步拉 URL 失败后任务被误判为取消。
- qBittorrent Web API 返回 401/403（例如 `/api/v2/app/version` 或 `/api/v2/torrents/add`）会归类为 `DOWNLOADER_AUTH_FAILED` / `status=unavailable`，不会返回 `INTERNAL_ERROR`。
- `POST /rss/items/:id/download` 手动入队如果在创建下载任务时失败，会把 item 持久化为 `needs_attention`，写入 `error_message` 和 `retry_reason`，前端可通过 `GET /rss/items?status=needs_attention` 看到失败原因；HTTP 响应仍使用稳定错误码（如下游认证失败返回 `503 DOWNLOADER_AUTH_FAILED`）。
- 每个订阅固定一个基础 `target_virtual_parent_path`；后端保存 VFS 解析快照 `target_vfs_parent_node_id`、`resolved_source_id`、`resolved_inner_parent_path`。
- RSS item 只有在关联下载任务为 `completed` 且任务带有非零 `result_vfs_node_id` 时才会变为 `completed`，并回写到 `RSSItemView.result_vfs_node_id` 方便前端从 RSS 条目跳转/定位 VFS 节点。
- `RSSSubscriptionView` / 创建更新请求新增：
  - `directory_template`：空值保持旧行为，RSS 入队仍使用 `target_virtual_parent_path`；非空时按条目 `parsed` 字段渲染为相对子目录，再拼到 `target_virtual_parent_path` 下。
  - `filename_template`：RSS 入队时会基于 item `parsed` / `title` 渲染为任务级 `target_filename` 快照；下载器仍写入 staging，后端仅在完成导入阶段对“单文件任务”重命名。模板结果不含明确扩展名（如 `.mkv` / `.mp4`；`S01.05` 这类集数后缀不算扩展名）时会自动保留原文件扩展名；多文件 torrent 保持原相对路径，不批量改名。
- 模板占位符支持 `{anime_title}`、`{season}`、`{episode}`、`{subtitle_group}`、`{resolution}`、`{title}`。目录模板会做路径安全清洗，禁止 `..`、绝对路径、Windows drive-style 前缀（如 `C:/anime` / `C:anime`）和反斜杠逃逸；未知占位符或非法模板返回 `400 PATH_INVALID`；条目解析字段缺失时对应占位渲染为空或安全 fallback，最终不会生成越界路径。
- 创建/更新订阅会校验目标 VFS 目录可解析、有 backing storage、当前用户具备写权限且底层源可写。
- RSS 条目去重优先使用 GUID；无 GUID 时使用 source + link + title 哈希。
- RSS 源会暴露无人值守调度字段：`health_status`（`ok` / `degraded` / `circuit_open`）、`consecutive_failures`、`last_success_at`、`next_refresh_at`、`last_refresh_status`、`last_refresh_stats`。
- RSS 源连续失败会记录 `last_error` 并按失败次数退避；超过阈值进入 `circuit_open`，成功刷新后恢复 `ok` 并清零失败计数。单源失败不会中断 refresh-all 中其他源；手动 refresh-all 会强制刷新所有启用源，`skipped` 仅表示该源已有刷新在进行。
- 条目状态当前可能为：`new`、`unsupported`、`ignored`、`matched`、`enqueued`、`retry_pending`、`completed`、`needs_attention`、`failed`。
- 条目重试字段：`retry_count`、`max_retry_count`、`last_attempt_at`、`next_retry_at`、`retry_reason`。自动重试默认最多 3 次，退避为 5m / 30m / 2h；手动 retry 可绕过 `next_retry_at`。
- 已有关联非终态 task 的 RSS item 不会重复入队；普通自动刷新不会绕过 `retry_pending` / `needs_attention` / `completed` 状态重复入队；task `completed` 且有 result node 才会回写 item `completed`，task `failed` / `canceled` 或 completed 但缺失 result node 会进入 `needs_attention`（或按既有瞬时下载器错误进入有限 `retry_pending`），metadata commit 失败固定展示安全错误摘要。
- 规则 preview 返回每个已有 item 的 `result`（`matched` / `missing` / `excluded`）以及 `matched`、`missing`、`excluded` 关键词列表，用于解释命中/未命中原因。
- 复制订阅会授权原订阅 owner，复制 `source_id`、规则、regex/case、目标路径、目录/文件名模板与 VFS 解析快照；新订阅使用新 `id/created_at/updated_at`。未传 `name` 时默认生成 `原名 Copy`（必要时简单追加序号），未传 `is_enabled` 时保持原订阅状态。
- 批量启用/禁用订阅对每个 `subscription_id` 独立 `FindSubscriptionByID` 与授权；成功项更新 `is_enabled` 与 `updated_at`，失败项写入 `error_code/error_message`，不会导致整个响应失败。
- 临时规则 preview 使用 `POST /rss/subscriptions/preview`，会校验 source 存在、当前身份可管理该 source，并校验正则；只基于该 source 现有 items 计算解释结果，返回 `subscription_id=0`，不会创建或更新订阅。
- 批量忽略对每个 `item_id` 独立 `FindItemByID` 与授权；已完成 item 或有关联非终态 task 的 item 返回单项失败 `TASK_INVALID_STATE`，其他可忽略项会设置 `status=ignored` 并清空 `error_message`、`retry_reason`、`next_retry_at`。
- 批量 retry 逐项复用单条 `POST /rss/items/:id/retry` 语义；`subscription_id` 若提供会应用到每个 item，单项失败写入该项 `error_code/error_message`，不会导致整个响应失败。
- qBittorrent 下载目录必须与 backend 共享；下载完成后仍由 Yunxia 从 staging 导入目标 VFS 目录；RSS 文件名模板只在这个导入阶段生效，不修改 qBittorrent 内部 torrent 内容。


### 3.11 notifications（`/api/v1/notifications`）

通知模块用于配置 Webhook 通道，并记录 RSS 无人值守运行过程中需要外部提醒或前端待处理展示的事件。当前真实通道只有 `webhook`；Telegram / 企业微信后续扩展时会复用 channel/event 模型。

| 方法 | 路径 | 权限 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/notifications/channels` | `notification.read` | - | 200，`{items[]}` |
| POST | `/notifications/channels` | `notification.manage` | `name,type,is_enabled,event_types,config` | 201，`{channel}` |
| PUT | `/notifications/channels/:id` | `notification.manage` | 同创建 | 200，`{channel}` |
| DELETE | `/notifications/channels/:id` | `notification.manage` | path: `id` | 200，`{deleted,id}` |
| POST | `/notifications/channels/:id/test` | `notification.manage` | path: `id` | 200，`{ok:true}` |
| GET | `/notifications/events` | `notification.read` | query: `status,event_type,limit` | 200，`{items[]}` |
| POST | `/notifications/events/:id/retry` | `notification.manage` | path: `id` | 202，`{event}` |

通道创建 / 更新请求体：

```json
{
  "name": "Ops Webhook",
  "type": "webhook",
  "is_enabled": true,
  "event_types": ["rss.source_failure", "rss.item_needs_attention", "rss.download_completed"],
  "config": {
    "url": "https://example.com/yunxia-webhook",
    "secret": "optional-signing-secret"
  }
}
```

字段说明：

- `type` 当前只支持 `webhook`。
- `is_enabled` 创建时省略默认为 `true`；更新时省略会保留原启用状态。
- `event_types=[]` 或省略表示接收全部支持事件。
- `config.secret` 只在创建/更新时提交；列表/详情不返回明文，只返回 `secret_configured`。更新时 `secret` 省略会保留旧值，传空字符串会清空。
- Webhook 请求为 `POST application/json`；配置 secret 时后端会追加：
  - `X-Yunxia-Timestamp`
  - `X-Yunxia-Signature: sha256=<hmac_sha256(timestamp + "." + body)>`

通道视图：

```json
{
  "id": 1,
  "name": "Ops Webhook",
  "type": "webhook",
  "is_enabled": true,
  "event_types": ["rss.source_failure"],
  "config": {
    "url": "https://example.com/yunxia-webhook",
    "secret_configured": true
  },
  "created_at": "2026-05-02T16:00:00+08:00",
  "updated_at": "2026-05-02T16:00:00+08:00"
}
```

当前支持事件类型：

| event_type | severity | 触发时机 |
|---|---|---|
| `rss.source_failure` | `warning` | RSS source 连续失败导致健康状态进入 `degraded` / `circuit_open` 时触发 |
| `rss.item_needs_attention` | `error` | RSS item 因确定性失败或重试耗尽进入 `needs_attention` 时触发 |
| `rss.download_completed` | `info` | RSS item 关联下载任务完成并回写为 `completed` 时触发 |

事件状态：

| status | 含义 |
|---|---|
| `pending` | 事件已入库，尚未投递 |
| `delivered` | 已成功投递到匹配通道 |
| `retry_pending` | 投递失败，等待自动/手动重试 |
| `failed` | 已达到自动重试上限 |
| `skipped` | 当前没有匹配的启用通道，事件只保留为记录 |

事件视图：

```json
{
  "id": 12,
  "user_id": 1,
  "event_type": "rss.item_needs_attention",
  "severity": "error",
  "title": "RSS item needs attention",
  "message": "file already exists",
  "payload": {
    "item_id": 7,
    "source_id": 2,
    "title": "Example S01E05",
    "retry_count": 3,
    "max_retry_count": 3
  },
  "status": "retry_pending",
  "attempts": 1,
  "max_attempts": 3,
  "last_attempt_at": "2026-05-02T16:00:00+08:00",
  "next_attempt_at": "2026-05-02T16:05:00+08:00",
  "delivered_at": null,
  "last_error": "webhook status 500",
  "created_at": "2026-05-02T16:00:00+08:00",
  "updated_at": "2026-05-02T16:00:00+08:00"
}
```

错误码：

| code | HTTP | 场景 |
|---|---:|---|
| `CONFIG_INVALID` | 422 | URL 非 http/https、未知 event_type、缺少 name 等配置错误 |
| `NOTIFICATION_CHANNEL_UNSUPPORTED` | 422 | 通道类型不是当前支持的 `webhook` |
| `NOTIFICATION_NOT_FOUND` | 404 | channel/event 不存在 |
| `NOTIFICATION_DELIVERY_FAILED` | 502 | 测试发送或手动重试时 webhook 返回非 2xx / 网络失败 |
| `TASK_INVALID_STATE` | 409 | 对 `delivered` / `skipped` 事件执行 retry |

### 3.12 shares（`/api/v1`）

| 方法 | 路径 | 鉴权 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/shares` | Bearer | - | 200，`{items[]}` |
| GET | `/shares/:id` | Bearer | path: `id` | 200，`{share}` |
| POST | `/shares` | Bearer | `source_id,path,expires_in,password` | 201，`{share}` |
| PUT | `/shares/:id` | Bearer | `expires_in,password` | 200，`{share}` |
| DELETE | `/shares/:id` | Bearer | path: `id` | 200，`{id,deleted}` |
| GET | `/s/:token` | 无 | query: `password,path,page,page_size,sort_by,sort_order,disposition` | 文件：302；目录：200 JSON；异常：401/404/410 |

补充：

- `ShareView` 当前包含快照字段：
  - `target_vfs_node_id`：分享目标对应的 metadata VFS node id；旧数据或未启用 metadata 解析时可能省略
  - `target_virtual_path`
  - `resolved_source_id`
  - `resolved_inner_path`
- 打开分享时后端优先按 `target_vfs_node_id` 解析当前 metadata VFS node；旧分享缺少 node id 时才兼容回退到 `target_virtual_path` / `source_id+path` 快照。
- 文件分享不会直接由 `/s/:token` 长时间流式返回大文件；成功校验分享密码/过期后返回 `302 Location: /api/v2/fs/download?path=<当前虚拟路径>&access_token=<短期令牌>&disposition=...`。短期令牌 TTL 默认为 5 分钟，且不会超过分享链接过期剩余时间。
- `/api/v2/fs/download` 会继续复用现有下载入口：local 返回文件流并支持 HTTP Range；S3/PikPak 等支持 presign 的 driver 返回 provider 临时 URL 302。
- rename / move 后分享继续跟随同一个 `target_vfs_node_id`；公开打开时使用 node 的当前 `path` 生成下载地址或目录列表。管理端列表中的 `target_virtual_path` / `resolved_inner_path` 仍是创建时快照字段，前端不要把它当成长期身份。
- 目标 node 被删除、标记 missing/error/conflict 等不可下载状态，或 node id 找不到时，公开打开返回 `404 FILE_NOT_FOUND`；分享自身不存在仍返回 `404 SHARE_NOT_FOUND`。
- 目录分享列表优先读取 metadata VFS children，并只返回 `PublicShareEntry` 字段（`name/path/parent_path/is_dir/preview_type/size/mime_type/...`），不会暴露 `source_id`、provider locator、provider file id 或底层路径；missing/error/pending/conflict 等不可用子节点不会出现在公开列表中。
- 普通用户默认仅能管理自己的分享
- 具备 `share.read_all` / `share.manage_all` capability 的角色可跨用户治理
- 目录分享的 `query.path` 是**相对分享根路径**
- 密码保护分享未带密码返回 `401 SHARE_PASSWORD_REQUIRED`
- 密码错误返回 `401 SHARE_PASSWORD_INVALID`
- 过期返回 `410 SHARE_EXPIRED`

### 3.13 audit（`/api/v1`）

| 方法 | 路径 | 权限 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/audit/logs` | `audit.read` | query: `page,page_size,actor_user_id,actor_role_key,resource_type,action,result,source_id,virtual_path,request_id,entrypoint,started_at,ended_at` | 200，`{items,total,page,page_size,total_pages}` |
| GET | `/audit/logs/:id` | `audit.read` | path: `id` | 200，审计详情 |

补充：

- `started_at` / `ended_at` 需要 RFC3339；非法时返回 `400 VALIDATION_ERROR`
- `entrypoint` 当前可能值：`rest_v1` / `rest_v2` / `webdav`
- 列表项中的 `summary` 是服务端生成的简短摘要
- 详情中的 `before` / `after` / `detail` 为可选对象，空值时会省略
- 当前即使拥有 `audit.read_sensitive`，也不会比 `audit.read` 看到更多明文字段

### 3.14 统一虚拟目录树 V2（`/api/v2`）

| 方法 | 路径 | 鉴权 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/fs/list` | Bearer | query: `path`，为空默认 `/` | 200，`{items,current_path}` |
| GET | `/fs/search` | Bearer | query: `path,keyword,page,page_size` | 200，`{items,path_prefix,keyword}` |
| POST | `/fs/refresh` | Bearer | `{path,mode}`，`mode` 当前只支持 `sync`（空值按 `sync` 处理） | 200，`VFSRefreshResponse` |
| POST | `/fs/mkdir` | Bearer | `parent_path,name` | 200，`{created}` |
| POST | `/fs/rename` | Bearer | `path,new_name` | 200，`{old_path,new_path,file}` |
| POST | `/fs/move` | Bearer | `path,target_path` | 200，`{old_path,new_path,moved}` |
| POST | `/fs/copy` | Bearer | `path,target_path` | 200，`{source_path,new_path,copied}` |
| DELETE | `/fs` | Bearer | `path,delete_mode` | 200，`{deleted,delete_mode,path,deleted_at}` |
| POST | `/fs/access-url` | Bearer | `path,purpose,disposition,expires_in` | 200，`{url,method,expires_at}` |
| GET | `/fs/download` | Bearer 或 `access_token` | query: `path,disposition[,access_token]` | local：200 文件流；S3/PikPak：302 |

补充：

- `VFSItem` 关键字段：
  - `id`: metadata VFS node id；旧式/未索引条目可能省略
  - `entry_kind`: `file` / `directory`
  - `is_virtual`
  - `is_mount_point`
  - `source_id`（纯虚拟节点时可能为空）
  - `sync_state`: `indexed` / `pending` / `syncing` / `stale` / `missing` / `conflict` / `error`；旧式/未索引条目可能省略
- `GET /api/v2/fs/download` 可作为分享和普通下载的统一数据面入口：传 `access_token` 时仍会先按当前 VFS path 解析 source/inner path，再校验 token 中的 source/path；local 文件由 Go `ServeContent` 处理 Range，支持 presign 的 provider 继续 302。
- `POST /fs/refresh` 请求示例：
  ```json
  { "path": "/pikpak/anime", "mode": "sync" }
  ```
- `mode` 省略或空字符串时按 `sync` 处理；非 `sync` 值会在 handler 层返回 `400 VALIDATION_ERROR`，不会触发 provider/list。
- `POST /fs/refresh` 成功响应 `data`：
  ```json
  {
    "path": "/pikpak/anime",
    "node_id": 42,
    "seen": 12,
    "indexed": 2,
    "updated": 1,
    "missing": 0,
    "conflicts": 0,
    "errors": 0,
    "sync_state": "indexed"
  }
  ```
- `/fs/list` 可能返回：
  - 实际文件 / 目录
  - 由 mount 组合出来的纯虚拟目录节点
- `/fs/list` 与 `/fs/search` 现在以 metadata VFS 读模型为准；进入挂载目录时后端会按需懒刷新当前目录的直接子项，再从 DB 返回列表 / 搜索结果
- `/fs/refresh` 是外部可调用的同步刷新入口，会在当前请求内刷新目标目录的直接子项，成功后再次调用 `/fs/list?path=<path>` 可看到最新 metadata 视图
- 懒刷新失败时后端优先保留已有 metadata 视图，避免 provider 短暂不可用导致目录完全不可读；不可下载 / 冲突 / 缺失状态的文件会通过 `can_download=false` 收敛给前端
- `/fs/refresh` 遇到 provider/list 失败时返回稳定错误（例如 `502 CLOUD_PROVIDER_UNAVAILABLE`），但不会清空已有 DB 子节点；目标目录会进入 `sync_state="error"`，旧列表视图仍可通过 `/fs/list` 读取
- `/fs/refresh` 发现同名或 provider identity 冲突时返回 `409 VFS_SYNC_CONFLICT`，相关节点会保留为 `sync_state="conflict"`，前端应提示需要人工处理或稍后重试
- `/fs/mkdir`、`/fs/rename`、`/fs/move`、`/fs/delete` 在底层 driver / file operator 成功后同步更新 metadata VFS 控制面；`/fs/copy` 在底层复制成功后刷新目标父目录，确保后续 `/fs/list` 以 metadata 视图立即可见
- 若底层写操作失败，metadata VFS 不会被修改；若底层已成功但 metadata 同步失败，接口返回 `500 METADATA_VFS_MUTATION_SYNC_FAILED`，响应 message 固定为 `metadata vfs mutation sync failed`，不会暴露物理路径、SQL 细节或 provider 原始 payload
- 后端会把上述“底层成功但 metadata 同步失败”的场景写入内部 `vfs_operations` operation journal，记录操作类型、路径/source 快照、安全错误码与下一次重试时间；本阶段不新增用户/前端可调用的管理 API，对外成功/失败语义不变
- 跨 source `move/copy` 本阶段只保留既有 local-to-local 同步能力；非 local 跨 source 仍返回稳定 `422 SOURCE_DRIVER_UNSUPPORTED` / `SOURCE_OPERATION_UNSUPPORTED`，不会伪装为全量 transfer 已完成
- 多用户开启后，`/fs/list?path=/` 只投影当前用户可见的挂载源；未授权 source 的挂载点名称不会出现在根目录
- `/fs/refresh` 至少要求当前用户对目标 path 有 read 权限；普通用户刷新未授权 path 返回 `403 ACL_DENIED` 或按不可见语义返回 `404 FILE_NOT_FOUND`，不会在错误消息中暴露挂载点/文件名
- 本地挂载目录探测为不可写时，列表项 `can_delete=false`；写操作返回 `403 SOURCE_READ_ONLY`
- 纯虚拟目录上的写操作（mkdir / rename / move / copy / delete / upload init）如果没有唯一 backing storage，返回 `409 NO_BACKING_STORAGE`
- 名称与挂载点冲突时返回 `409 NAME_CONFLICT`
- `/fs/access-url` 当前会返回 `/api/v2/fs/download?...&access_token=...`

### 3.15 VFS 标签（`/api/v1/tags`、`/api/v2/fs/tags`）

| 方法 | 路径 | 鉴权 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/api/v1/tags` | Bearer | - | 200，`{items}` |
| POST | `/api/v1/tags` | Bearer | `{name,color}` | 201，`{tag}` |
| PATCH | `/api/v1/tags/:id` | Bearer | `{name,color}` | 200，`{tag}` |
| DELETE | `/api/v1/tags/:id` | Bearer | path: `id` | 200，`{deleted,id}` |
| GET | `/api/v2/fs/tags` | Bearer | query: `path` | 200，`{path,tags}` |
| POST | `/api/v2/fs/tags/attach` | Bearer | `{path,tag_id}` | 200，`{path,tags}` |
| POST | `/api/v2/fs/tags/detach` | Bearer | `{path,tag_id}` | 200，`{path,tags}` |

`VFSTagView`：

```json
{
  "id": 1,
  "name": "番剧",
  "color": "#66ccff",
  "created_at": "2026-05-07T12:00:00Z",
  "updated_at": "2026-05-07T12:00:00Z"
}
```

约束：

- 标签按当前登录用户隔离；普通 Bearer 用户只能管理自己的 tag。
- `name` 必填，后端会 trim，最长 64 字符；`color` 可空，最长 32 字符。
- 同一用户同名标签 `POST /api/v1/tags` 会幂等 upsert。
- 节点标签绑定基于 metadata VFS node；如果目标 path 尚未被懒索引，需要前端先进入对应目录触发 `/api/v2/fs/list`。
- 绑定他人的 tag 返回 `403 PERMISSION_DENIED`；节点不存在返回 `404 FILE_NOT_FOUND`。

### 3.16 WebDAV

支持方法：

- `OPTIONS`
- `HEAD`
- `GET`
- `PUT`
- `DELETE`
- `PROPFIND`
- `MKCOL`
- `COPY`
- `MOVE`

路由模式：

- `{webdav_prefix}/:slug`
- `{webdav_prefix}/:slug/*filepath`

约束：

- 使用 Basic Auth
- 对 `is_webdav_exposed=true` 的存储源开放；local / S3 / PikPak 等所有 source 都统一通过 metadata VFS 控制面进入，不再让 local 绕到物理 `webdav.Dir`，也不让非 local 暴露 provider 原生路径
- 需要 HTTPS 语义；反向代理场景应传 `X-Forwarded-Proto: https`
- 普通用户仍受 ACL 约束
- `webdav_read_only=true` 时写方法会被拒绝
- metadata VFS 行为：
  - `PROPFIND` 走 `VFSService.List` / metadata parent stat，返回 WebDAV `207 Multi-Status`；服务端先按 metadata VFS + ACL 过滤，普通用户不会收到未授权节点名称
  - `GET` / `HEAD` 先按 WebDAV slug 合成 VFS path，再通过 VFS resolve 生成 `/api/v2/fs/download?path=...&access_token=...` 短链 302；后续由统一 VFS download 入口处理 local Range 流或 provider presign/302
  - `MKCOL` / `DELETE` / `COPY` / `MOVE` 调用 `VFSService.Mkdir/Delete/Copy/Move/Rename`，复用网页 VFS 的 metadata mutation sync、operation journal 与 ACL 行为
  - `PUT` 先把请求体写入后端临时文件，再按 VFS resolve 得到的 source/inner parent 调用 `UploadService.ImportLocalFile`；返回 `201` 前必须完成数据面导入与 metadata VFS file/object commit
  - `COPY` / `MOVE` 的 `Destination` 必须仍在同一个 WebDAV source slug 下；跨 WebDAV source 移动/复制当前不支持
- 只读与防泄露：
  - `webdav_read_only=true` 时 `PUT` / `MKCOL` / `DELETE` / `COPY` / `MOVE` 稳定返回 `403`，响应只包含通用 WebDAV 错误文本，不返回容器/宿主机物理路径
  - metadata sync / commit 失败返回 5xx WebDAV 错误状态，响应不包含 SQL、provider payload、local 物理路径或 token
- 写方法当前会写入审计：
  - `PUT -> file.put`
  - `MKCOL -> file.mkcol`
  - `DELETE -> file.delete`
  - `COPY -> file.copy`
  - `MOVE -> file.move`
- WebDAV 写操作审计结果按 HTTP 状态归类：
  - `2xx/3xx -> success`
  - `4xx -> denied`
  - `5xx -> failed`

## 4. 关键结构示例

### 4.1 CurrentUserResponse

```json
{
  "user": {
    "id": 1,
    "username": "admin",
    "email": "admin@example.com",
    "role_key": "super_admin",
    "status": "active",
    "created_at": "2026-04-23T15:00:00+08:00"
  },
  "capabilities": [
    "system.stats.read",
    "source.read"
  ]
}
```

### 4.2 StorageSourceView

```json
{
  "id": 1,
  "name": "本地存储",
  "driver_type": "local",
  "status": "online",
  "is_enabled": true,
  "is_webdav_exposed": false,
  "webdav_read_only": true,
  "webdav_slug": "local",
  "mount_path": "/local",
  "root_path": "/",
  "used_bytes": 0,
  "total_bytes": null,
  "created_at": "2026-04-23T15:00:00+08:00",
  "updated_at": "2026-04-23T15:00:00+08:00"
}
```

### 4.3 UploadSessionView（含虚拟路径快照）

```json
{
  "upload_id": "upl_xxx",
  "source_id": 1,
  "path": "/",
  "filename": "brief.txt",
  "file_size": 5,
  "file_hash": "5d41402abc4b2a76b9719d911017c592",
  "chunk_size": 5242880,
  "total_chunks": 1,
  "uploaded_chunks": [],
  "status": "uploading",
  "is_fast_upload": false,
  "expires_at": "2026-04-30T12:00:00+08:00",
  "target_vfs_parent_node_id": 12,
  "target_virtual_parent_path": "/docs",
  "resolved_source_id": 1,
  "resolved_inner_parent_path": "/"
}
```

### 4.4 DownloadTaskView（含虚拟路径快照）

```json
{
  "id": 1,
  "type": "download",
  "downloader_type": "aria2",
  "status": "pending",
  "source_id": 1,
  "save_path": "/downloads",
  "target_vfs_parent_node_id": 12,
  "target_virtual_parent_path": "/local/downloads",
  "target_filename": "Example Show - S01E05 [1080p]",
  "save_virtual_path": "/local/downloads",
  "resolved_source_id": 1,
  "resolved_inner_save_path": "/downloads",
  "result_vfs_node_id": 34,
  "display_name": "archive.zip",
  "source_url": "https://example.com/archive.zip",
  "progress": 0,
  "downloaded_bytes": 0,
  "total_bytes": null,
  "speed_bytes": 0,
  "eta_seconds": null,
  "error_message": null,
  "created_at": "2026-04-23T15:00:00+08:00",
  "updated_at": "2026-04-23T15:00:00+08:00",
  "finished_at": null
}
```

### 4.5 ShareView（含虚拟路径快照）

```json
{
  "id": 1,
  "source_id": 1,
  "path": "/docs/hello.txt",
  "target_vfs_node_id": 42,
  "target_virtual_path": "/local/docs/hello.txt",
  "resolved_source_id": 1,
  "resolved_inner_path": "/docs/hello.txt",
  "name": "hello.txt",
  "is_dir": false,
  "link": "/s/uuid-token",
  "has_password": false,
  "expires_at": null,
  "created_at": "2026-04-23T15:00:00+08:00"
}
```

说明：`target_vfs_node_id` 是长期追踪身份；`path`、`target_virtual_path`、`resolved_inner_path` 是兼容/展示快照。公开打开分享时，后端会按 node 当前路径生成目录响应或 `/api/v2/fs/download` 302，因此 rename/move 后公开访问可能看到新的当前路径。

### 4.6 VFSItem

```json
{
  "id": 123,
  "name": "team",
  "path": "/docs/team",
  "parent_path": "/docs",
  "source_id": null,
  "entry_kind": "directory",
  "is_virtual": true,
  "is_mount_point": true,
  "sync_state": "indexed",
  "size": 0,
  "mime_type": "",
  "extension": "",
  "modified_at": "",
  "created_at": "",
  "etag": "",
  "can_preview": false,
  "can_download": false,
  "can_delete": false
}
```

### 4.7 AuditLogDetailResponse

```json
{
  "id": 12,
  "occurred_at": "2026-04-23T16:00:00+08:00",
  "actor": {
    "user_id": 1,
    "username": "admin",
    "role_key": "super_admin"
  },
  "request": {
    "request_id": "req_xxx",
    "entrypoint": "webdav",
    "client_ip": "192.0.2.1",
    "user_agent": "",
    "method": "MKCOL",
    "path": "/dav/local/docs"
  },
  "target": {
    "source_id": 1,
    "virtual_path": "/local/docs"
  },
  "resource_type": "file",
  "action": "mkcol",
  "result": "success",
  "summary": "file.mkcol.success",
  "after": {
    "virtual_path": "/local/docs"
  },
  "detail": {
    "status": 201,
    "request_path": "/docs",
    "target_virtual_path": "/local/docs"
  }
}
```

## 5. 当前实际错误码

### 5.1 auth / permission

- `AUTH_TOKEN_MISSING`
- `AUTH_TOKEN_INVALID`
- `AUTH_ACCOUNT_LOCKED`
- `AUTH_INVALID_CREDENTIALS`
- `AUTH_REFRESH_TOKEN_INVALID`
- `CAPABILITY_DENIED`
- `ACL_DENIED`
- `PERMISSION_DENIED`
- `ROLE_ASSIGNMENT_FORBIDDEN`
- `LAST_SUPER_ADMIN_FORBIDDEN`

### 5.2 setup / user / acl

- `SETUP_ALREADY_COMPLETED`
- `USER_NOT_FOUND`
- `USER_NAME_CONFLICT`
- `USER_ROLE_INVALID`
- `USER_STATUS_INVALID`
- `ACL_RULE_NOT_FOUND`
- `VFS_NODE_NOT_FOUND`
- `ACL_SUBJECT_TYPE_INVALID`
- `ACL_EFFECT_INVALID`
- `ACL_PERMISSIONS_INVALID`

### 5.3 source / config / mount

- `SOURCE_NOT_FOUND`
- `SOURCE_DRIVER_UNSUPPORTED`
- `SOURCE_OPERATION_UNSUPPORTED`
- `SOURCE_CONNECTION_FAILED`
- `SOURCE_NAME_CONFLICT`
- `SOURCE_IN_USE`
- `SOURCE_READ_ONLY`
- `CONFIG_INVALID`
- `CLOUD_AUTH_FAILED`
- `CLOUD_TOKEN_INVALID`
- `CLOUD_CAPTCHA_REQUIRED`
- `CLOUD_RATE_LIMITED`
- `CLOUD_REGION_BLOCKED`
- `CLOUD_PROVIDER_UNAVAILABLE`
- `MOUNT_PATH_CONFLICT`
- `METADATA_VFS_MOUNT_SYNC_FAILED`
- `PATH_INVALID`

### 5.4 file / upload / trash / vfs

- `FILE_NOT_FOUND`
- `FILE_ALREADY_EXISTS`
- `FILE_NAME_INVALID`
- `FILE_MOVE_CONFLICT`
- `FILE_COPY_CONFLICT`
- `FILE_IS_DIRECTORY`
- `NAME_CONFLICT`
- `NO_BACKING_STORAGE`
- `VFS_SYNC_CONFLICT`
- `UPLOAD_SESSION_NOT_FOUND`
- `UPLOAD_CHUNK_CONFLICT`
- `UPLOAD_FINISH_INCOMPLETE`
- `UPLOAD_HASH_MISMATCH`
- `UPLOAD_INVALID_STATE`
- `UPLOAD_TOO_LARGE`
- `TRASH_ITEM_NOT_FOUND`
- `TRASH_RESTORE_CONFLICT`
- `METADATA_VFS_COMMIT_FAILED`
- `METADATA_VFS_MUTATION_SYNC_FAILED`
- `TAG_INVALID`
- `TAG_NOT_FOUND`
- `TAG_BINDING_NOT_FOUND`

### 5.5 share / task

- `SHARE_NOT_FOUND`
- `SHARE_PASSWORD_REQUIRED`
- `SHARE_PASSWORD_INVALID`
- `SHARE_EXPIRED`
- `TASK_NOT_FOUND`
- `TASK_INVALID_STATE`
- `DOWNLOADER_UNAVAILABLE`
- `DOWNLOADER_AUTH_FAILED`
- `DOWNLOAD_LINK_UNSUPPORTED`
- `FILE_NAME_INVALID`（任务创建传入非法 `target_filename`）

### 5.6 rss

- `RSS_SOURCE_NOT_FOUND`（源不存在；导入订阅的 `source_url` 无法映射到当前 owner 的已有/本次导入 source 时作为单项 `error_code` 返回）
- `RSS_SUBSCRIPTION_NOT_FOUND`（订阅不存在；批量启停中作为单项 `error_code` 返回）
- `RSS_ITEM_NOT_FOUND`
- `RSS_QBITTORRENT_UNAVAILABLE`
- `RSS_REGEX_INVALID`
- `CONFIG_INVALID`（导入 source / subscription 字段不合法时作为单项 `error_code` 返回）
- `PATH_INVALID`（订阅目标目录或模板非法；导入时作为单项 `error_code` 返回）
- `NO_BACKING_STORAGE` / `SOURCE_READ_ONLY` / `PERMISSION_DENIED`（导入订阅目标不可写时作为单项 `error_code` 返回）
- `DOWNLOAD_LINK_UNSUPPORTED`
- `DOWNLOADER_UNAVAILABLE` / `DOWNLOADER_AUTH_FAILED`（RSS 手动/批量入队时 qBittorrent 不可用或下游认证失败）
- `TASK_INVALID_STATE`（批量忽略中 item 已完成或存在活跃任务等不可变更状态）

### 5.7 audit

- `AUDIT_LOG_NOT_FOUND`

### 5.8 通用

- `VALIDATION_ERROR`
- `INTERNAL_ERROR`

## 6. 当前与前端联调最容易踩坑的点

1. `GET /api/v1/system/version` 不是公开接口，必须登录。
2. `GET /api/v1/files/download` 与 `GET /api/v2/fs/download` 都是公开路由，但**仍必须**携带 Bearer 或 `access_token`。
3. `GET /api/v1/sources?view=navigation` 只要求登录，不要求 `source.read` capability。
4. `GET /api/v1/tasks/:id` 返回的是**直接任务对象**，不是 `{task: ...}`。
5. `DELETE /api/v1/upload/sessions/:upload_id` 返回的是 `{upload_id,canceled}`，不是空对象。
6. `DELETE /api/v1/acl/rules/:id` 返回的是 `{}`，不是 `{deleted,id}`。
7. 上传初始化已支持 `target_virtual_parent_path`，且优先级高于 `source_id/path`。
8. 离线下载创建任务也已支持 `target_virtual_parent_path`；前端推荐传当前 VFS 目录作为目标父目录。
9. 离线下载默认先落 backend 与下载器共享的 staging，完成后由后端导入 local / S3 / PikPak；但目标解析到 PikPak source 时会优先使用 `pikpak_native` provider 原生离线下载，完成后文件已在 PikPak 中，不再走 staging。
10. RSS 订阅第一版只自动处理 `magnet:?` 和 `.torrent`，并要求 qBittorrent 可用；普通 HTTP RSS 条目不会自动入队。
11. `/api/v2/fs/list` / `/api/v2/fs/search` 已切到 metadata VFS 读模型，并会按 ACL 过滤挂载点、纯虚拟父目录和真实子项；前端不要自行展示后端未返回的文件。
12. ACL 配置页优先使用 `/api/v2/fs/list` 返回的 `VFSItem.id` 作为 `vfs_node_id` 创建规则；旧 `source_id + path` 仍兼容但只作为 path 快照 fallback。
13. `mount_path` 已是存储源模型的一部分，默认本地源当前挂载在 `/local`。
14. 分享公开下载现在先 302 到 `/api/v2/fs/download?path=<当前 VFS path>&access_token=...`；前端/浏览器直接跳转即可，不要用 JSON client 解析，也不要把创建时 `target_virtual_path` 当成 rename/move 后的真实路径。
15. 当前已经存在并可用的统一虚拟目录接口：`/api/v2/fs/*`。
16. 审计查询接口当前已经存在：`GET /api/v1/audit/logs`、`GET /api/v1/audit/logs/:id`，并要求 `audit.read`。
17. `audit.read_sensitive` 目前只是预留能力位，前端不要基于它假设会返回更多敏感字段。
18. WebDAV 写操作当前也会落审计，但审计失败不会影响主请求成功状态。
19. RSS 条目批量动作返回 `RSSItemBatchActionResponse.items[]`，每项都有 `success` 与可选 `item/error_code/error_message`；不要按单条接口的 `{item}` 包装解析，也不要因为 HTTP 200/202 就假设全部成功。
20. RSS 订阅批量启停返回 `RSSSubscriptionBatchStateResponse.items[]`，每项都有 `subscription_id/success` 与可选 `subscription/error_code/error_message`；复制订阅返回 201 `{subscription}`，不会修改原订阅。
21. RSS 导入返回 `RSSImportResponse`，HTTP 200 只代表请求已处理；前端必须检查 `sources.items[]`、`subscriptions.items[]` 和 `failed` 计数。`dry_run=true` 的 `created` 是“将创建”数量，不代表已落库。
22. 通知事件 HTTP 200/202 只代表请求处理成功；Webhook 投递结果要看 `NotificationEventView.status`。`event_types=[]` 表示通道接收全部支持事件。

## 7. 前端常见页面调用流程

### 7.1 应用启动流程

1. `GET /api/v1/setup/status`
2. 如果 `setup_required=true`：进入初始化页，提交 `POST /api/v1/setup/init`
3. 如果已初始化但没有本地 token：进入登录页，提交 `POST /api/v1/auth/login`
4. 登录成功保存：
   - `tokens.access_token`
   - `tokens.refresh_token`
   - `user`
5. 进入主应用后调用 `GET /api/v1/auth/me` 刷新当前用户与 `capabilities`

### 7.2 文件管理页推荐流程（VFS）

1. 初始化目录树：`GET /api/v2/fs/list?path=/`
2. 点击目录：`GET /api/v2/fs/list?path=<item.path>`
3. 用户点击刷新按钮：`POST /api/v2/fs/refresh { "path": "<current_path>", "mode": "sync" }`，成功后重新调用 `GET /api/v2/fs/list?path=<current_path>`
4. 新建目录：`POST /api/v2/fs/mkdir`
5. 上传文件：
   - `POST /api/v1/upload/init`，传 `target_virtual_parent_path=<current_path>`
   - local：`PUT /api/v1/upload/chunk` 后 `POST /api/v1/upload/finish`
   - S3：按 `part_instructions` 直传后 `POST /api/v1/upload/finish`
   - PikPak：优先按 `transport.mode` 分支；能提供 GCID 时可走 `direct_parts` OSS PUT，不能提供时走 `server_chunk` 后端导入
6. 下载文件：
   - `POST /api/v2/fs/access-url`
   - 浏览器打开返回的 `url`
7. 删除文件：
   - `DELETE /api/v2/fs`，默认 `delete_mode=trash`
   - PikPak 删除会调用 provider `batchTrash`，语义是移入 PikPak 回收站；不要在 UI 中标注为永久删除
8. 回收站：
   - 如果页面是按 source 展示回收站，使用 `/api/v1/trash?source_id=<id>`

### 7.3 存储源管理页流程

1. 管理视图列表：`GET /api/v1/sources?view=admin`
2. 创建前测试配置：`POST /api/v1/sources/test`
3. 创建：`POST /api/v1/sources`
4. 详情编辑：`GET /api/v1/sources/:id`
5. 保存：`PUT /api/v1/sources/:id`
6. 删除：`DELETE /api/v1/sources/:id`

### 7.4 分享页流程

1. 分享列表：`GET /api/v1/shares`
2. 创建分享：`POST /api/v1/shares`
3. 更新分享：`PUT /api/v1/shares/:id`
4. 删除分享：`DELETE /api/v1/shares/:id`
5. 公开分享页：
   - 目录：`GET /s/:token?path=/&password=xxx`
   - 文件：直接打开 `/s/:token?password=xxx`

### 7.5 管理权限渲染建议

前端按钮显示建议以 capability 为准：

| 页面动作 | capability |
|---|---|
| 查看系统统计 | `system.stats.read` |
| 查看系统配置 | `system.config.read` |
| 修改系统配置 | `system.config.write` |
| 查看用户 | `user.read` |
| 创建用户 | `user.create` + `user.role.assign` |
| 更新用户 | `user.update` + `user.role.assign` + `user.lock` |
| 重置密码 | `user.password.reset` |
| 撤销用户令牌 | `user.tokens.revoke` |
| 查看 ACL | `acl.read` |
| 管理 ACL | `acl.manage` |
| 查看 source 管理列表/详情 | `source.read` |
| 测试 source | `source.test` |
| 创建 source | `source.create` |
| 更新 source | `source.update` |
| 删除 source | `source.delete` |
| 查看审计 | `audit.read` |
