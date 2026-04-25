# Yunxia Backend API Contract

> 更新时间：2026-04-25
> 对应实现：当前工作树 `backend/` 实际代码（含全局权限模型 + 统一虚拟目录树 V2 + 审计查询）
> 真相源：`backend/internal/interfaces/http/router.go`、`backend/internal/interfaces/http/handler/*.go`、`backend/internal/application/dto/*.go`、`backend/internal/application/service/*_service.go`

本文档只描述**当前后端实际实现**，用于前后端联调、API client 封装与页面功能核对。

## 0. 前端接入速览

### 0.1 当前后端模块总览

| 模块 | Base | 当前能力 | 前端用途 |
|---|---|---|---|
| 初始化 / 认证 | `/api/v1` | 初始化、登录、刷新 token、登出、当前用户能力 | 登录页、初始化页、全局权限渲染 |
| 系统 | `/api/v1/system/*` | version、stats、config 读写 | 管理后台首页、系统设置页 |
| 用户管理 | `/api/v1/users*` | 用户列表、创建、更新、重置密码、撤销令牌 | 用户管理页 |
| ACL | `/api/v1/acl/rules*` | 对用户授予/拒绝 source 内路径权限 | 权限配置页 |
| 存储源 | `/api/v1/sources*` | local/S3 源列表、详情、创建、更新、删除、测试 | 存储源管理、侧边栏导航 |
| 传统文件 | `/api/v1/files*` | 按 `source_id + path` 管理文件 | 兼容旧文件页 |
| 上传 | `/api/v1/upload*` | 初始化、分片、完成、会话、取消 | 文件上传 |
| 回收站 | `/api/v1/trash*` | 列表、恢复、永久删除、清空 | 回收站页 |
| 离线任务 | `/api/v1/tasks*` | 创建、列表、详情、暂停、恢复、取消 | 离线任务页 |
| 分享 | `/api/v1/shares*`、`/s/:token` | 分享管理、公开分享访问 | 分享管理页、公开分享页 |
| 审计 | `/api/v1/audit/logs*` | 审计列表、审计详情 | 审计日志页 |
| 统一虚拟目录 V2 | `/api/v2/fs*` | 基于虚拟路径的文件列表、搜索、写操作、下载 | **新文件管理页推荐优先使用** |
| WebDAV | `{webdav_prefix}` 默认 `/dav` | WebDAV 客户端访问 local 源 | 前端主要展示配置，不直接走 JSON API |

### 0.2 新文件管理页推荐接口

如果是新写前端文件管理页面，推荐优先用 VFS v2，不要让页面直接关心底层 source 类型：

| 页面动作 | 推荐接口 |
|---|---|
| 进入根目录 | `GET /api/v2/fs/list?path=/` |
| 进入目录 | `GET /api/v2/fs/list?path=/local/docs` |
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

下载接口不要用上面的 JSON 封装；local 会返回文件流，S3 会返回 302。最简单的调用方式：

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
| `GET /api/v1/files/download` | local：200 文件流；S3：302 到 presigned URL |
| `GET /api/v2/fs/download` | local：200 文件流；S3：302 到 presigned URL |
| `GET /s/:token` | 文件：302 到下载地址；目录：200 JSON |
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
| `operator` | 只读统计、源读取/测试、跨用户任务治理；**当前没有**跨用户分享治理 capability |
| `user` | 无治理 capability；主要依赖 ACL 访问数据面 |

### 2.4 当前关键规则

- 初始化首用户固定创建为 `super_admin`
- 禁止移除最后一个激活的 `super_admin`
- `GET /api/v1/sources?view=navigation` 只要求登录；结果会按 ACL 过滤
- `view=admin` / source 详情 / source 增删改测：按 capability 控制
- `task` / `share`：owner 默认可管理自己的数据；具备跨用户 capability 的角色可跨用户治理
- S3 明文 secret 仅 `source.secret.read` 可见；当前仅 `super_admin` 可见
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
| GET | `/acl/rules` | `acl.read` | query: `source_id,path` | 200，`{items[]}` |
| POST | `/acl/rules` | `acl.manage` | `source_id,path,subject_type,subject_id,effect,priority,permissions,inherit_to_children` | 201，`{rule}` |
| PUT | `/acl/rules/:id` | `acl.manage` | `path,subject_type,subject_id,effect,priority,permissions,inherit_to_children` | 200，`{rule}` |
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

补充：

- 初始化完成后自动创建默认本地源：`本地存储`
- 默认本地源当前挂载到 `mount_path=/local`
- `mount_path` 需要全局唯一，冲突返回 `409 MOUNT_PATH_CONFLICT`
- `PUT /sources/:id` 当前会保留原有 `driver_type`，不是切换驱动接口
- `GET /sources/:id` 对 S3 源返回 `secret_fields`；只有 `super_admin` 可看到 `config.access_key / config.secret_key` 明文

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
| GET | `/files/download` | Bearer 或 `access_token` | query: `source_id,path,disposition[,access_token]` | local：200 文件流；S3：302 |

补充：

- `delete_mode` 为空时默认按 `trash`
- 数据面接口会做 ACL 校验；失败返回 `403 ACL_DENIED`
- `files/access-url` 对 local / S3 当前都先返回应用内短链 `/api/v1/files/download?...&access_token=...`
- 真正的 S3 presigned URL 在 `GET /files/download` 时再 302 跳转

### 3.7 upload（`/api/v1`）

| 方法 | 路径 | 鉴权 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| POST | `/upload/init` | Bearer | 两种模式，见下方 | 200，`UploadInitResponse` |
| PUT | `/upload/chunk` | Bearer | query: `upload_id,index`，body 为二进制分片 | 200，`{upload_id,index,received_bytes,already_uploaded}` |
| POST | `/upload/finish` | Bearer | `upload_id[,parts[]]` | 201，`{completed,upload_id,file}` |
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
  - `target_virtual_parent_path`
  - `resolved_source_id`
  - `resolved_inner_parent_path`
- 本地源返回 `transport.mode=server_chunk`
- S3 源返回 multipart 直传说明 `part_instructions[]`
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
| POST | `/tasks` | Bearer | `type,url,source_id,save_path` | 202，`{task}` |
| GET | `/tasks/:id` | Bearer | path: `id` | 200，**直接返回 `DownloadTaskView`** |
| POST | `/tasks/:id/pause` | Bearer | path: `id` | 200，`{id,status}` |
| POST | `/tasks/:id/resume` | Bearer | path: `id` | 200，`{id,status}` |
| DELETE | `/tasks/:id` | Bearer | query: `delete_file` | 200，`{id,canceled,delete_file}` |

补充：

- `save_path` 入参仍是 **source 内部路径**，不是虚拟目录路径
- 返回体当前会补充 VFS 快照字段：
  - `save_virtual_path`
  - `resolved_source_id`
  - `resolved_inner_save_path`
- 普通用户默认仅能看到 / 操作自己的任务
- 具备 `task.read_all` / `task.manage_all` capability 的角色可跨用户治理
- ACL / 权限失败统一返回 `403 PERMISSION_DENIED`
- 当前没有 `retry` 接口

### 3.10 shares（`/api/v1`）

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
  - `target_virtual_path`
  - `resolved_source_id`
  - `resolved_inner_path`
- 普通用户默认仅能管理自己的分享
- 具备 `share.read_all` / `share.manage_all` capability 的角色可跨用户治理
- 目录分享的 `query.path` 是**相对分享根路径**
- 密码保护分享未带密码返回 `401 SHARE_PASSWORD_REQUIRED`
- 密码错误返回 `401 SHARE_PASSWORD_INVALID`
- 过期返回 `410 SHARE_EXPIRED`

### 3.11 audit（`/api/v1`）

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

### 3.12 统一虚拟目录树 V2（`/api/v2`）

| 方法 | 路径 | 鉴权 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/fs/list` | Bearer | query: `path`，为空默认 `/` | 200，`{items,current_path}` |
| GET | `/fs/search` | Bearer | query: `path,keyword,page,page_size` | 200，`{items,path_prefix,keyword}` |
| POST | `/fs/mkdir` | Bearer | `parent_path,name` | 200，`{created}` |
| POST | `/fs/rename` | Bearer | `path,new_name` | 200，`{old_path,new_path,file}` |
| POST | `/fs/move` | Bearer | `path,target_path` | 200，`{old_path,new_path,moved}` |
| POST | `/fs/copy` | Bearer | `path,target_path` | 200，`{source_path,new_path,copied}` |
| DELETE | `/fs` | Bearer | `path,delete_mode` | 200，`{deleted,delete_mode,path,deleted_at}` |
| POST | `/fs/access-url` | Bearer | `path,purpose,disposition,expires_in` | 200，`{url,method,expires_at}` |
| GET | `/fs/download` | Bearer 或 `access_token` | query: `path,disposition[,access_token]` | local：200 文件流；S3：302 |

补充：

- `VFSItem` 关键字段：
  - `entry_kind`: `file` / `directory`
  - `is_virtual`
  - `is_mount_point`
  - `source_id`（纯虚拟节点时可能为空）
- `/fs/list` 可能返回：
  - 实际文件 / 目录
  - 由 mount 组合出来的纯虚拟目录节点
- 纯虚拟目录上的写操作（mkdir / rename / move / copy / delete / upload init）如果没有唯一 backing storage，返回 `409 NO_BACKING_STORAGE`
- 名称与挂载点冲突时返回 `409 NAME_CONFLICT`
- `/fs/access-url` 当前会返回 `/api/v2/fs/download?...&access_token=...`

### 3.13 WebDAV

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
- 仅对 `is_webdav_exposed=true` 的 local 源开放
- 需要 HTTPS 语义；反向代理场景应传 `X-Forwarded-Proto: https`
- 普通用户仍受 ACL 约束
- `webdav_read_only=true` 时写方法会被拒绝
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
  "status": "pending",
  "source_id": 1,
  "save_path": "/downloads",
  "save_virtual_path": "/local/downloads",
  "resolved_source_id": 1,
  "resolved_inner_save_path": "/downloads",
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

### 4.6 VFSItem

```json
{
  "name": "team",
  "path": "/docs/team",
  "parent_path": "/docs",
  "source_id": null,
  "entry_kind": "directory",
  "is_virtual": true,
  "is_mount_point": true,
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
- `ACL_SUBJECT_TYPE_INVALID`
- `ACL_EFFECT_INVALID`
- `ACL_PERMISSIONS_INVALID`

### 5.3 source / config / mount

- `SOURCE_NOT_FOUND`
- `SOURCE_DRIVER_UNSUPPORTED`
- `SOURCE_CONNECTION_FAILED`
- `SOURCE_NAME_CONFLICT`
- `SOURCE_IN_USE`
- `CONFIG_INVALID`
- `MOUNT_PATH_CONFLICT`
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
- `UPLOAD_SESSION_NOT_FOUND`
- `UPLOAD_CHUNK_CONFLICT`
- `UPLOAD_FINISH_INCOMPLETE`
- `UPLOAD_HASH_MISMATCH`
- `UPLOAD_INVALID_STATE`
- `UPLOAD_TOO_LARGE`
- `TRASH_ITEM_NOT_FOUND`
- `TRASH_RESTORE_CONFLICT`

### 5.5 share / task

- `SHARE_NOT_FOUND`
- `SHARE_PASSWORD_REQUIRED`
- `SHARE_PASSWORD_INVALID`
- `SHARE_EXPIRED`
- `TASK_NOT_FOUND`
- `TASK_INVALID_STATE`
- `DOWNLOADER_UNAVAILABLE`

### 5.6 audit

- `AUDIT_LOG_NOT_FOUND`

### 5.7 通用

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
8. `mount_path` 已是存储源模型的一部分，默认本地源当前挂载在 `/local`。
9. 当前已经存在并可用的统一虚拟目录接口：`/api/v2/fs/*`。
10. 审计查询接口当前已经存在：`GET /api/v1/audit/logs`、`GET /api/v1/audit/logs/:id`，并要求 `audit.read`。
11. `audit.read_sensitive` 目前只是预留能力位，前端不要基于它假设会返回更多敏感字段。
12. WebDAV 写操作当前也会落审计，但审计失败不会影响主请求成功状态。

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
3. 新建目录：`POST /api/v2/fs/mkdir`
4. 上传文件：
   - `POST /api/v1/upload/init`，传 `target_virtual_parent_path=<current_path>`
   - local：`PUT /api/v1/upload/chunk` 后 `POST /api/v1/upload/finish`
   - S3：按 `part_instructions` 直传后 `POST /api/v1/upload/finish`
5. 下载文件：
   - `POST /api/v2/fs/access-url`
   - 浏览器打开返回的 `url`
6. 删除文件：
   - `DELETE /api/v2/fs`，默认 `delete_mode=trash`
7. 回收站：
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
