# 前后端 API 契约文档 —— 云匣 (Yunxia)

> **版本**: v0.1  
> **日期**: 2026-04-21  
> **用途**: 前后端联调 / Mock / 实际开发落地的接口真相源  
> **覆盖范围**: `P0 Stable` + `P1 Draft/Reserved`  
> **对应文档**: `PRD.md` / `TAD.md` / `DESIGN.md` / `FRONTEND-DESIGN.md` / `INTERFACE-ARCHITECTURE.md`

---

## 0. 文档角色与效力

### 0.1 文档角色

本文件定义云匣项目在前后端协作阶段的 API 真相源，覆盖：

- Web UI ↔ REST API
- WebDAV 协议接口
- 统一响应结构、错误码、分页/排序、鉴权、上传下载、Mock 与联调规则

### 0.2 冲突处理顺序

当本文件与其他设计文档存在 API 细节冲突时，前后端联调阶段按以下顺序处理：

1. 本 API 契约文档
2. `DOCS-INDEX.md` 的统一约定
3. `PRD.md` 的产品边界
4. `TAD.md` 的架构与已有路由真值表
5. `DESIGN.md` / `FRONTEND-DESIGN.md` 的实现草图与页面说明

### 0.3 状态标签

- **Stable**：后端承诺按此实现；前端可直接依赖；变更必须显式记录
- **Draft**：前端可据此做页面、Mock、状态流；后端尽量兼容，但允许小范围调整
- **Reserved**：只冻结资源名、路由方向或能力边界；不承诺字段最终形态

### 0.4 本文档采用的分层策略

- **P0 主链路 API**：尽量收敛为 `Stable`
- **前端可能提前开发的 P1 能力**：收敛为 `Draft`
- **仅需防命名漂移的能力**：收敛为 `Reserved`

---

## 1. 范围与总体规则

### 1.1 P0 Stable 覆盖范围

本文件将以下能力定义为 `Stable`：

- 启动 / 初始化 / 认证
- 文件管理主链路
- 上传主链路（断点续传 / 秒传 / 会话恢复）
- 存储源管理
- 离线下载任务主链路
- 系统配置与版本信息
- WebDAV 协议接口

### 1.2 P1 Draft / Reserved 覆盖范围

以下能力不进入 P0 Stable，但为避免前端被阻塞，收敛为 `Draft` 或 `Reserved`：

- 用户管理
- ACL 管理
- 回收站管理
- 下载暂停 / 恢复
- 系统统计
- 分享
- 全文搜索 / 索引任务
- 审计日志

### 1.3 新增并冻结的契约补充项

相较于现有技术文档，本文件补充并冻结以下能力：

- `GET /api/v1/setup/status`
- `POST /api/v1/setup/init`
- `POST /api/v1/files/rename`
- `POST /api/v1/files/access-url`
- `GET /api/v1/upload/sessions`
- `DELETE /api/v1/upload/sessions/:upload_id`
- `POST /api/v1/sources/test`
- `StorageSource.webdav_slug`

这些项自本文件起，视为前后端联调阶段的正式契约。

---

## 2. 全局统一约定

### 2.1 Base Path

#### REST API

```text
/api/v1
```

#### WebDAV

```text
/dav
```

### 2.2 REST API 统一响应结构

#### 成功响应

```json
{
  "success": true,
  "code": "OK",
  "message": "ok",
  "data": {},
  "meta": {
    "request_id": "req_01HSXYZ...",
    "timestamp": "2026-04-21T11:30:45+08:00"
  }
}
```

#### 失败响应

```json
{
  "success": false,
  "code": "AUTH_TOKEN_EXPIRED",
  "message": "access token expired",
  "error": {
    "details": {
      "field": "authorization"
    }
  },
  "meta": {
    "request_id": "req_01HSXYZ...",
    "timestamp": "2026-04-21T11:30:45+08:00"
  }
}
```

### 2.3 统一结构原则

- HTTP Status 表示协议层结果
- `body.code` 表示业务语义，供前端做精细判断
- `body.message` 用于人类可读提示
- 成功响应总带 `data`
- 失败响应总带 `error`
- 所有响应必须带 `meta.request_id` 与 `meta.timestamp`

### 2.4 例外接口

以下接口不强制使用统一 JSON 包装：

#### 文件下载
- `GET /api/v1/files/download`
- 成功返回文件流；失败返回 JSON 错误体

#### 上传 chunk
- `PUT /api/v1/upload/chunk`
- 请求体为二进制 chunk；成功响应仍返回 JSON

#### WebDAV
- `/dav/*`
- 完全遵循 WebDAV 协议；不套 REST JSON 包装

### 2.5 HTTP 状态码约定

| HTTP Status | 用途 |
|---|---|
| `200 OK` | 普通查询、更新成功 |
| `201 Created` | 创建成功 |
| `202 Accepted` | 异步任务已受理 |
| `204 No Content` | 删除成功且无需内容 |
| `206 Partial Content` | Range 下载 |
| `400 Bad Request` | 参数格式错误 |
| `401 Unauthorized` | 未登录、Token 无效 |
| `403 Forbidden` | 有身份但无权限 |
| `404 Not Found` | 资源不存在 |
| `409 Conflict` | 冲突、重复、状态不允许 |
| `422 Unprocessable Entity` | 参数合法但业务不可处理 |
| `429 Too Many Requests` | 限流 |
| `500 Internal Server Error` | 服务端异常 |
| `503 Service Unavailable` | 依赖服务不可用 |

### 2.6 统一错误码表

#### 通用类
- `OK`
- `BAD_REQUEST`
- `VALIDATION_ERROR`
- `INTERNAL_ERROR`
- `RATE_LIMITED`
- `FEATURE_NOT_ENABLED`

#### 初始化 / 认证类
- `SETUP_REQUIRED`
- `SETUP_ALREADY_COMPLETED`
- `AUTH_INVALID_CREDENTIALS`
- `AUTH_TOKEN_MISSING`
- `AUTH_TOKEN_INVALID`
- `AUTH_TOKEN_EXPIRED`
- `AUTH_REFRESH_TOKEN_INVALID`
- `AUTH_ACCOUNT_LOCKED`

#### 权限类
- `PERMISSION_DENIED`
- `ACL_DENIED`
- `ROLE_FORBIDDEN`

#### 文件类
- `FILE_NOT_FOUND`
- `FILE_ALREADY_EXISTS`
- `FILE_NAME_INVALID`
- `FILE_MOVE_CONFLICT`
- `FILE_COPY_CONFLICT`
- `FILE_IS_DIRECTORY`
- `PATH_INVALID`

#### 上传类
- `UPLOAD_SESSION_NOT_FOUND`
- `UPLOAD_CHUNK_OUT_OF_RANGE`
- `UPLOAD_CHUNK_CONFLICT`
- `UPLOAD_HASH_MISMATCH`
- `UPLOAD_FINISH_INCOMPLETE`
- `UPLOAD_TOO_LARGE`
- `UPLOAD_INVALID_STATE`

#### 存储源类
- `SOURCE_NOT_FOUND`
- `SOURCE_DRIVER_UNSUPPORTED`
- `SOURCE_CONNECTION_FAILED`
- `SOURCE_READ_ONLY`
- `SOURCE_NAME_CONFLICT`
- `SOURCE_IN_USE`

#### 任务 / 下载器类
- `TASK_NOT_FOUND`
- `TASK_INVALID_STATE`
- `TASK_CREATE_FAILED`
- `DOWNLOADER_UNAVAILABLE`

#### 配置类
- `CONFIG_INVALID`

### 2.7 全局字段命名规则

#### JSON 命名
- 一律使用 `snake_case`

#### 布尔字段
- 优先使用：
  - `is_*`
  - `has_*`
  - `can_*`

#### ID
- 资源 ID 对前端暴露为 `integer`
- 上传会话 / request id 等流程标识使用 `string`

#### 时间
- 一律使用 RFC3339 带时区
- 示例：`2026-04-21T11:30:45+08:00`

#### 文件大小
- 一律使用 bytes 的整数

#### 路径
- 必须以 `/` 开头
- 根目录固定为 `/`
- 除根目录外，不以 `/` 结尾
- 后端负责路径净化；前端不得拼接 `..`

#### 枚举值
- 一律使用小写蛇形

### 2.8 分页 / 排序 / 查询规则

#### 分页
```http
?page=1&page_size=200
```

响应中的分页信息放在：

```json
"meta": {
  "request_id": "req_xxx",
  "timestamp": "2026-04-21T11:30:45+08:00",
  "pagination": {
    "page": 1,
    "page_size": 200,
    "total": 1532,
    "total_pages": 8
  }
}
```

#### 排序
```http
?sort_by=modified_at&sort_order=desc
```

#### 搜索
- 统一使用 `keyword`
- 不混用 `q` / `query` / `search`

---
## 3. 核心资源模型

> 以下为前后端交换的资源视图模型，不等同于数据库表结构。

### 3.1 Stable 资源模型

#### 3.1.1 `SetupStatus`

```json
{
  "is_initialized": false,
  "setup_required": true,
  "has_admin": false
}
```

#### 3.1.2 `UserSummary`

```json
{
  "id": 1,
  "username": "admin",
  "email": "admin@example.com",
  "role": "admin",
  "is_locked": false,
  "created_at": "2026-04-21T11:30:45+08:00"
}
```

字段说明：
- `role`: `admin | normal | guest`
- 不暴露 `password`、`token_version`、失败计数等内部字段

#### 3.1.3 `AuthTokenPair`

```json
{
  "access_token": "eyJhbGciOi...",
  "refresh_token": "eyJhbGciOi...",
  "expires_in": 900,
  "refresh_expires_in": 604800,
  "token_type": "Bearer"
}
```

#### 3.1.4 `StorageSource`

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
  "root_path": "/",
  "used_bytes": 1073741824,
  "total_bytes": 5368709120,
  "created_at": "2026-04-21T11:30:45+08:00",
  "updated_at": "2026-04-21T11:30:45+08:00"
}
```

字段说明：
- `driver_type`: `local | s3 | onedrive`
- `status`: `online | offline | error`
- `webdav_slug`：用于 WebDAV 路径映射，创建后应尽量稳定

#### 3.1.5 `FileItem`

```json
{
  "name": "movie.mp4",
  "path": "/videos/movie.mp4",
  "parent_path": "/videos",
  "source_id": 1,
  "is_dir": false,
  "size": 1073741824,
  "mime_type": "video/mp4",
  "extension": ".mp4",
  "etag": "md5:abcd1234",
  "modified_at": "2026-04-21T11:30:45+08:00",
  "created_at": "2026-04-20T10:00:00+08:00",
  "can_preview": true,
  "can_download": true,
  "can_delete": true,
  "thumbnail_url": null
}
```

字段说明：
- 目录项固定：`is_dir=true`，`size=0`，`mime_type="inode/directory"`
- `can_*` 为当前用户视角的操作能力

#### 3.1.6 `FileListResult`

```json
{
  "items": [],
  "current_path": "/videos",
  "current_source_id": 1
}
```

#### 3.1.7 `UploadSession`

```json
{
  "upload_id": "upl_01HSXYZ...",
  "source_id": 1,
  "path": "/uploads",
  "filename": "archive.zip",
  "file_size": 20971520,
  "file_hash": "md5hex",
  "chunk_size": 5242880,
  "total_chunks": 4,
  "uploaded_chunks": [0, 1],
  "status": "uploading",
  "is_fast_upload": false,
  "expires_at": "2026-04-28T11:30:45+08:00"
}
```

状态枚举：`pending | uploading | completed | canceled | expired`

#### 3.1.8 `DownloadTask`

```json
{
  "id": 1001,
  "type": "download",
  "status": "running",
  "source_id": 1,
  "save_path": "/downloads",
  "display_name": "movie.mkv",
  "source_url": "magnet:?xt=urn:btih:...",
  "progress": 45.2,
  "downloaded_bytes": 1288490188,
  "total_bytes": 2852126720,
  "speed_bytes": 2411724,
  "eta_seconds": 720,
  "error_message": null,
  "created_at": "2026-04-21T11:30:45+08:00",
  "updated_at": "2026-04-21T11:35:12+08:00",
  "finished_at": null
}
```

状态枚举：`pending | running | paused | completed | failed | canceled`

#### 3.1.9 `SystemConfigPublic`

```json
{
  "site_name": "云匣",
  "multi_user_enabled": false,
  "default_source_id": 1,
  "max_upload_size": 10737418240,
  "default_chunk_size": 5242880,
  "webdav_enabled": true,
  "webdav_prefix": "/dav",
  "theme": "system",
  "language": "zh-CN",
  "time_zone": "Asia/Shanghai"
}
```

### 3.2 Draft 资源模型

#### 3.2.1 `ManagedUser`

```json
{
  "id": 2,
  "username": "alice",
  "email": "alice@example.com",
  "role": "normal",
  "status": "active",
  "last_login_at": "2026-04-21T10:20:00+08:00",
  "created_at": "2026-04-15T09:00:00+08:00"
}
```

#### 3.2.2 `ACLRule`

```json
{
  "id": 301,
  "source_id": 1,
  "path": "/projects",
  "subject_type": "user",
  "subject_id": 2,
  "effect": "allow",
  "priority": 100,
  "permissions": {
    "read": true,
    "write": true,
    "delete": false,
    "share": false
  },
  "inherit_to_children": true
}
```

#### 3.2.3 `TrashItem`

```json
{
  "id": 501,
  "source_id": 1,
  "original_path": "/docs/report.pdf",
  "trash_path": "/.trash/2026/04/report.pdf",
  "name": "report.pdf",
  "size": 2457600,
  "deleted_at": "2026-04-21T11:30:45+08:00",
  "expires_at": "2026-05-21T11:30:45+08:00"
}
```

### 3.3 Reserved 资源模型

- `ShareLink`
- `SearchIndexJob`
- `AuditLogEntry`
- `BackupJob`

Reserved 资源仅冻结资源名与能力方向，不冻结字段。

---

## 4. P0 Stable REST API

## 4.1 启动 / 初始化 / 认证

### 4.1.1 `GET /api/v1/health`
- **状态**：`Stable`
- **认证**：公开
- **用途**：健康检查、Docker healthcheck

成功响应：

```json
{
  "success": true,
  "code": "OK",
  "message": "ok",
  "data": {
    "status": "ok",
    "service": "yunxia",
    "version": "1.0.0"
  },
  "meta": {
    "request_id": "req_xxx",
    "timestamp": "2026-04-21T11:30:45+08:00"
  }
}
```

### 4.1.2 `GET /api/v1/setup/status`
- **状态**：`Stable`
- **认证**：公开
- **用途**：前端启动路由分流

成功响应 `data`：

```json
{
  "is_initialized": false,
  "setup_required": true,
  "has_admin": false
}
```

前端启动顺序建议：
1. `GET /api/v1/setup/status`
2. 若 `setup_required=true` → 跳 `/setup`
3. 若无需 setup 且本地有 token → 调 `GET /api/v1/auth/me`
4. 否则跳 `/login`

### 4.1.3 `POST /api/v1/setup/init`
- **状态**：`Stable`
- **认证**：公开，仅首次初始化可用
- **用途**：创建首个管理员账号并直接登录

请求体：

```json
{
  "username": "admin",
  "password": "strong-password-123",
  "email": "admin@example.com"
}
```

成功响应 `data`：

```json
{
  "user": {
    "id": 1,
    "username": "admin",
    "email": "admin@example.com",
    "role": "admin",
    "is_locked": false,
    "created_at": "2026-04-21T11:30:45+08:00"
  },
  "tokens": {
    "access_token": "eyJhbGciOi...",
    "refresh_token": "eyJhbGciOi...",
    "expires_in": 900,
    "refresh_expires_in": 604800,
    "token_type": "Bearer"
  }
}
```

错误码：
- `VALIDATION_ERROR`
- `SETUP_ALREADY_COMPLETED`
- `INTERNAL_ERROR`

HTTP 状态：
- 成功：`201 Created`
- 已完成初始化：`409 Conflict`

### 4.1.4 `POST /api/v1/auth/login`
- **状态**：`Stable`
- **认证**：公开
- **用途**：用户名密码登录

请求体：

```json
{
  "username": "admin",
  "password": "strong-password-123"
}
```

成功响应 `data` 结构与 `setup/init` 中的 `user + tokens` 一致。

错误码：
- `VALIDATION_ERROR`
- `AUTH_INVALID_CREDENTIALS`
- `AUTH_ACCOUNT_LOCKED`
- `SETUP_REQUIRED`
- `RATE_LIMITED`

说明：
- 登录接口独立限流
- 账户锁定建议返回：`403 Forbidden + AUTH_ACCOUNT_LOCKED`

### 4.1.5 `POST /api/v1/auth/refresh`
- **状态**：`Stable`
- **认证**：公开（通过 refresh token）
- **用途**：刷新访问令牌

请求体：

```json
{
  "refresh_token": "eyJhbGciOi..."
}
```

成功响应 `data`：

```json
{
  "tokens": {
    "access_token": "new_access_token",
    "refresh_token": "new_refresh_token",
    "expires_in": 900,
    "refresh_expires_in": 604800,
    "token_type": "Bearer"
  }
}
```

规则：
- 使用 refresh token rotation
- 刷新成功后旧 refresh token 立即失效

错误码：
- `VALIDATION_ERROR`
- `AUTH_REFRESH_TOKEN_INVALID`
- `AUTH_TOKEN_EXPIRED`
- `AUTH_ACCOUNT_LOCKED`

### 4.1.6 `POST /api/v1/auth/logout`
- **状态**：`Stable`
- **认证**：`Bearer access_token`
- **用途**：登出当前设备会话

请求头：
```http
Authorization: Bearer <access_token>
```

请求体：

```json
{
  "refresh_token": "eyJhbGciOi..."
}
```

成功响应 `data`：

```json
{}
```

说明：
- 服务端撤销当前 `refresh_token`
- 不承担“全设备登出”语义

### 4.1.7 `GET /api/v1/auth/me`
- **状态**：`Stable`
- **认证**：`Bearer access_token`
- **用途**：恢复当前登录用户态

成功响应 `data`：`UserSummary`

错误码：
- `AUTH_TOKEN_MISSING`
- `AUTH_TOKEN_INVALID`
- `AUTH_TOKEN_EXPIRED`

---
## 4.2 文件管理

### 4.2.0 统一约束
- 所有文件操作以单资源为单位
- P0 不提供 Stable 批量 API
- 多选批量操作由前端拆成多个单文件请求
- 所有接口均需 Bearer Token + ACL 校验
- 默认不暴露系统内部目录，如 `/.trash`、`/.system`

### 4.2.1 `GET /api/v1/files`
- **状态**：`Stable`
- **用途**：列出某存储源某目录下的文件与目录

Query 参数：
```http
GET /api/v1/files?source_id=1&path=/videos&page=1&page_size=200&sort_by=modified_at&sort_order=desc
```

参数：
- `source_id`：必填
- `path`：必填
- `page`：默认 `1`
- `page_size`：默认 `200`
- `sort_by`：`name | size | modified_at`
- `sort_order`：`asc | desc`

成功响应 `data`：

```json
{
  "items": [
    {
      "name": "movie.mp4",
      "path": "/videos/movie.mp4",
      "parent_path": "/videos",
      "source_id": 1,
      "is_dir": false,
      "size": 1073741824,
      "mime_type": "video/mp4",
      "extension": ".mp4",
      "etag": "md5:abcd1234",
      "modified_at": "2026-04-21T11:30:45+08:00",
      "created_at": "2026-04-20T10:00:00+08:00",
      "can_preview": true,
      "can_download": true,
      "can_delete": true,
      "thumbnail_url": null
    }
  ],
  "current_path": "/videos",
  "current_source_id": 1
}
```

`meta.pagination`：

```json
{
  "page": 1,
  "page_size": 200,
  "total": 1532,
  "total_pages": 8
}
```

说明：
- 默认目录优先，再按排序字段排序
- 仅列当前目录，不递归
- 返回结果已完成 ACL 过滤

### 4.2.2 `GET /api/v1/files/search`
- **状态**：`Stable`
- **用途**：按文件名搜索当前存储源内资源

Query 参数：
```http
GET /api/v1/files/search?source_id=1&keyword=movie&page=1&page_size=50
```

可选参数：
- `path_prefix`

成功响应 `data`：

```json
{
  "items": [],
  "keyword": "movie",
  "current_source_id": 1,
  "path_prefix": null
}
```

说明：
- Stable 阶段只承诺“文件名模糊搜索”
- 不承诺全文搜索

### 4.2.3 `POST /api/v1/files/mkdir`
- **状态**：`Stable`
- **用途**：在指定目录下创建文件夹

请求体：

```json
{
  "source_id": 1,
  "parent_path": "/videos",
  "name": "new-folder"
}
```

成功响应 `data`：

```json
{
  "created": {
    "name": "new-folder",
    "path": "/videos/new-folder",
    "parent_path": "/videos",
    "source_id": 1,
    "is_dir": true,
    "size": 0,
    "mime_type": "inode/directory",
    "extension": "",
    "etag": "",
    "modified_at": "2026-04-21T11:30:45+08:00",
    "created_at": "2026-04-21T11:30:45+08:00",
    "can_preview": false,
    "can_download": false,
    "can_delete": true,
    "thumbnail_url": null
  }
}
```

说明：
- `name` 不允许包含 `/`
- 不支持自动级联创建多级目录

### 4.2.4 `POST /api/v1/files/rename`
- **状态**：`Stable`
- **用途**：重命名单个文件或目录

请求体：

```json
{
  "source_id": 1,
  "path": "/videos/movie.mp4",
  "new_name": "movie-1080p.mp4"
}
```

成功响应 `data`：

```json
{
  "old_path": "/videos/movie.mp4",
  "new_path": "/videos/movie-1080p.mp4",
  "file": {
    "name": "movie-1080p.mp4",
    "path": "/videos/movie-1080p.mp4",
    "parent_path": "/videos",
    "source_id": 1,
    "is_dir": false,
    "size": 1073741824,
    "mime_type": "video/mp4",
    "extension": ".mp4",
    "etag": "md5:abcd1234",
    "modified_at": "2026-04-21T11:30:45+08:00",
    "created_at": "2026-04-20T10:00:00+08:00",
    "can_preview": true,
    "can_download": true,
    "can_delete": true,
    "thumbnail_url": null
  }
}
```

说明：
- 仅支持同目录改名
- 不承担移动语义

### 4.2.5 `POST /api/v1/files/move`
- **状态**：`Stable`
- **用途**：把单个资源移动到目标目录

请求体：

```json
{
  "source_id": 1,
  "path": "/videos/movie.mp4",
  "target_path": "/archive"
}
```

成功响应 `data`：

```json
{
  "old_path": "/videos/movie.mp4",
  "new_path": "/archive/movie.mp4",
  "moved": true
}
```

说明：
- `target_path` 必须是目标目录
- Stable 阶段仅支持同一存储源内移动

### 4.2.6 `POST /api/v1/files/copy`
- **状态**：`Stable`
- **用途**：复制单个资源到目标目录

请求体：

```json
{
  "source_id": 1,
  "path": "/videos/movie.mp4",
  "target_path": "/backup"
}
```

成功响应 `data`：

```json
{
  "source_path": "/videos/movie.mp4",
  "new_path": "/backup/movie.mp4",
  "copied": true
}
```

说明：
- Stable 阶段仅支持同一存储源内复制

### 4.2.7 `DELETE /api/v1/files`
- **状态**：`Stable`
- **用途**：删除单个文件或目录

请求体：

```json
{
  "source_id": 1,
  "path": "/videos/movie.mp4",
  "delete_mode": "trash"
}
```

字段：
- `delete_mode`: `trash | permanent`
- 默认：`trash`

成功响应 `data`：

```json
{
  "deleted": true,
  "delete_mode": "trash",
  "path": "/videos/movie.mp4",
  "deleted_at": "2026-04-21T11:30:45+08:00"
}
```

说明：
- P0 默认删除行为为进回收站
- 回收站独立管理接口见 Draft 区

### 4.2.8 `GET /api/v1/files/download`
- **状态**：`Stable`
- **用途**：文件下载、媒体流读取

Query 参数：
```http
GET /api/v1/files/download?source_id=1&path=/videos/movie.mp4&disposition=attachment
```

字段：
- `source_id`
- `path`
- `disposition`: `attachment | inline`，默认 `attachment`

成功时返回：
- 文件流
- `Content-Type`
- `Content-Length`
- `Content-Disposition`
- `Accept-Ranges: bytes`
- `ETag`
- `Last-Modified`

说明：
- 支持 Range 请求
- 目录请求下载必须返回 `FILE_IS_DIRECTORY`

### 4.2.9 `POST /api/v1/files/access-url`
- **状态**：`Stable`
- **用途**：生成短时效访问地址，供原生浏览器标签预览或直接下载

请求体：

```json
{
  "source_id": 1,
  "path": "/videos/movie.mp4",
  "purpose": "preview",
  "disposition": "inline",
  "expires_in": 300
}
```

成功响应 `data`：

```json
{
  "url": "https://example.com/api/v1/files/download?access_token=temp_xxx",
  "method": "GET",
  "expires_at": "2026-04-21T11:35:45+08:00"
}
```

字段：
- `purpose`: `preview | download`
- `disposition`: `inline | attachment`
- `expires_in`: 秒，默认建议 `300`

说明：
- 前端预览和下载优先依赖此接口，而非自行拼接 `/download`
- 后端可按驱动类型返回代理地址或预签名地址

### 4.2.10 文件相关错误码
- `VALIDATION_ERROR`
- `SOURCE_NOT_FOUND`
- `PATH_INVALID`
- `FILE_NOT_FOUND`
- `FILE_ALREADY_EXISTS`
- `FILE_NAME_INVALID`
- `FILE_MOVE_CONFLICT`
- `FILE_COPY_CONFLICT`
- `FILE_IS_DIRECTORY`
- `PERMISSION_DENIED`
- `SOURCE_READ_ONLY`

---
## 4.3 上传

### 4.3.0 统一约束
上传统一抽象为两种模式：
- `server_chunk`：由云匣后端接收 chunk，适用于 `local`
- `direct_parts`：前端直接 PUT 到临时地址，适用于 `s3` / `onedrive`

前端统一流程：
1. `POST /api/v1/upload/init`
2. 根据 `transport.mode` 选择上传方式
3. 所有 chunk/part 完成后 `POST /api/v1/upload/finish`

### 4.3.1 `POST /api/v1/upload/init`
- **状态**：`Stable`
- **用途**：初始化上传、命中秒传、恢复未完成上传、获取直传说明

请求体：

```json
{
  "source_id": 1,
  "path": "/uploads",
  "filename": "archive.zip",
  "file_size": 20971520,
  "file_hash": "md5hex",
  "last_modified_at": "2026-04-21T10:00:00+08:00"
}
```

#### 命中秒传时成功响应 `data`

```json
{
  "is_fast_upload": true,
  "file": {
    "name": "archive.zip",
    "path": "/uploads/archive.zip",
    "parent_path": "/uploads",
    "source_id": 1,
    "is_dir": false,
    "size": 20971520,
    "mime_type": "application/zip",
    "extension": ".zip",
    "etag": "md5:abcd1234",
    "modified_at": "2026-04-21T11:30:45+08:00",
    "created_at": "2026-04-21T11:30:45+08:00",
    "can_preview": false,
    "can_download": true,
    "can_delete": true,
    "thumbnail_url": null
  }
}
```

#### 普通上传 / 断点续传成功响应 `data`

```json
{
  "is_fast_upload": false,
  "upload": {
    "upload_id": "upl_01HSXYZ...",
    "source_id": 1,
    "path": "/uploads",
    "filename": "archive.zip",
    "file_size": 20971520,
    "file_hash": "md5hex",
    "chunk_size": 5242880,
    "total_chunks": 4,
    "uploaded_chunks": [0, 1],
    "status": "uploading",
    "is_fast_upload": false,
    "expires_at": "2026-04-28T11:30:45+08:00"
  },
  "transport": {
    "mode": "server_chunk",
    "driver_type": "local",
    "concurrency": 3,
    "retry_limit": 3
  },
  "part_instructions": []
}
```

#### 直传模式成功响应 `data`

```json
{
  "is_fast_upload": false,
  "upload": {
    "upload_id": "upl_01HSXYZ...",
    "source_id": 2,
    "path": "/uploads",
    "filename": "archive.zip",
    "file_size": 20971520,
    "file_hash": "md5hex",
    "chunk_size": 5242880,
    "total_chunks": 4,
    "uploaded_chunks": [0],
    "status": "uploading",
    "is_fast_upload": false,
    "expires_at": "2026-04-28T11:30:45+08:00"
  },
  "transport": {
    "mode": "direct_parts",
    "driver_type": "s3",
    "concurrency": 3,
    "retry_limit": 3
  },
  "part_instructions": [
    {
      "index": 1,
      "method": "PUT",
      "url": "https://storage.example.com/presigned-part-1",
      "headers": {},
      "byte_range": {
        "start": 5242880,
        "end": 10485759
      },
      "expires_at": "2026-04-21T11:35:45+08:00"
    }
  ]
}
```

`part_instructions` 字段冻结为：
- `index`
- `method`
- `url`
- `headers`
- `byte_range.start`
- `byte_range.end`
- `expires_at`

#### Init 幂等与恢复规则
若存在同一用户、同一目标、同一文件大小/hash 的未完成上传会话：
- 返回原 `upload_id`
- 返回最新 `uploaded_chunks`
- 对直传模式重新签发剩余 chunk 的 `part_instructions`

#### 错误码
- `VALIDATION_ERROR`
- `SOURCE_NOT_FOUND`
- `PATH_INVALID`
- `FILE_NAME_INVALID`
- `FILE_ALREADY_EXISTS`
- `PERMISSION_DENIED`
- `SOURCE_READ_ONLY`
- `UPLOAD_TOO_LARGE`

### 4.3.2 `PUT /api/v1/upload/chunk`
- **状态**：`Stable`
- **用途**：本地磁盘场景下上传单个 chunk
- **仅在** `transport.mode=server_chunk` 时使用

请求：
```http
PUT /api/v1/upload/chunk?upload_id=upl_01HSXYZ...&index=2
Authorization: Bearer <access_token>
Content-Type: application/octet-stream
```

成功响应 `data`：

```json
{
  "upload_id": "upl_01HSXYZ...",
  "index": 2,
  "received_bytes": 5242880,
  "already_uploaded": false
}
```

幂等规则：
- 同一 `upload_id + index` 若已成功写入，再次上传可返回 `200` 且 `already_uploaded=true`
- 若内容冲突，返回 `UPLOAD_CHUNK_CONFLICT`

### 4.3.3 `POST /api/v1/upload/finish`
- **状态**：`Stable`
- **用途**：所有 chunk 完成后通知后端合并或提交最终文件

本地请求体：

```json
{
  "upload_id": "upl_01HSXYZ..."
}
```

直传模式请求体：

```json
{
  "upload_id": "upl_01HSXYZ...",
  "parts": [
    {
      "index": 0,
      "etag": "\"abc-part-etag\""
    },
    {
      "index": 1,
      "etag": "\"def-part-etag\""
    }
  ]
}
```

成功响应 `data`：

```json
{
  "completed": true,
  "upload_id": "upl_01HSXYZ...",
  "file": {
    "name": "archive.zip",
    "path": "/uploads/archive.zip",
    "parent_path": "/uploads",
    "source_id": 1,
    "is_dir": false,
    "size": 20971520,
    "mime_type": "application/zip",
    "extension": ".zip",
    "etag": "md5:abcd1234",
    "modified_at": "2026-04-21T11:35:45+08:00",
    "created_at": "2026-04-21T11:35:45+08:00",
    "can_preview": false,
    "can_download": true,
    "can_delete": true,
    "thumbnail_url": null
  }
}
```

HTTP 状态：`201 Created`

错误码：
- `VALIDATION_ERROR`
- `UPLOAD_SESSION_NOT_FOUND`
- `UPLOAD_FINISH_INCOMPLETE`
- `UPLOAD_HASH_MISMATCH`
- `FILE_ALREADY_EXISTS`
- `SOURCE_NOT_FOUND`
- `PERMISSION_DENIED`

### 4.3.4 `GET /api/v1/upload/sessions`
- **状态**：`Stable`
- **用途**：获取当前用户活动上传会话，用于恢复上传面板状态

Query 参数：
```http
GET /api/v1/upload/sessions?status=uploading&source_id=1
```

成功响应 `data`：

```json
{
  "items": [
    {
      "upload_id": "upl_01HSXYZ...",
      "source_id": 1,
      "path": "/uploads",
      "filename": "archive.zip",
      "file_size": 20971520,
      "file_hash": "md5hex",
      "chunk_size": 5242880,
      "total_chunks": 4,
      "uploaded_chunks": [0, 1],
      "status": "uploading",
      "is_fast_upload": false,
      "expires_at": "2026-04-28T11:30:45+08:00"
    }
  ]
}
```

关键现实语义：
- 后端可以恢复上传会话状态
- 浏览器刷新后不一定能自动恢复本地文件句柄
- 前端必要时应提示用户重新选择同一文件继续上传

### 4.3.5 `DELETE /api/v1/upload/sessions/:upload_id`
- **状态**：`Stable`
- **用途**：取消上传并清理服务端会话

成功响应 `data`：

```json
{
  "upload_id": "upl_01HSXYZ...",
  "canceled": true
}
```

错误码：
- `UPLOAD_SESSION_NOT_FOUND`
- `UPLOAD_INVALID_STATE`
- `PERMISSION_DENIED`

### 4.3.6 上传暂停 / 恢复约定
- P0 不提供独立 Stable 的 pause/resume 后端接口
- 前端暂停 = 停止本地调度
- 前端恢复 = 继续剩余 chunk 或重新调用 `upload/init`

---

## 4.4 存储源管理

### 4.4.0 统一约束
- 读取：普通用户仅能读取自己可见的存储源
- 管理：仅 `admin`
- 删除存储源 = 删除云匣中的挂载配置，不删除底层真实数据
- 敏感 secret 字段永不明文回显

### 4.4.1 `GET /api/v1/sources`
- **状态**：`Stable`
- **用途**：列出存储源，用于侧边栏与管理页

Query 参数：
```http
GET /api/v1/sources?view=navigation
GET /api/v1/sources?view=admin
```

参数：
- `view`: `navigation | admin`
- 默认：`navigation`

成功响应 `data`：

```json
{
  "items": [
    {
      "id": 1,
      "name": "本地存储",
      "driver_type": "local",
      "status": "online",
      "is_enabled": true,
      "is_webdav_exposed": false,
      "webdav_read_only": true,
      "webdav_slug": "local",
      "root_path": "/",
      "used_bytes": 1073741824,
      "total_bytes": 5368709120,
      "created_at": "2026-04-21T11:30:45+08:00",
      "updated_at": "2026-04-21T11:30:45+08:00"
    }
  ],
  "view": "navigation"
}
```

说明：
- `navigation` 仅返回启用且当前用户可见的存储源
- `admin` 仅管理员可用，并返回全部存储源

### 4.4.2 `GET /api/v1/sources/:id`
- **状态**：`Stable`
- **权限**：仅 `admin`
- **用途**：获取单个存储源详情

成功响应 `data`：

```json
{
  "source": {
    "id": 2,
    "name": "媒体仓库",
    "driver_type": "s3",
    "status": "online",
    "is_enabled": true,
    "is_webdav_exposed": true,
    "webdav_read_only": false,
    "webdav_slug": "media",
    "root_path": "/",
    "used_bytes": null,
    "total_bytes": null,
    "created_at": "2026-04-21T11:30:45+08:00",
    "updated_at": "2026-04-21T11:30:45+08:00"
  },
  "config": {
    "endpoint": "https://s3.example.com",
    "region": "us-east-1",
    "bucket": "media",
    "force_path_style": true
  },
  "secret_fields": {
    "access_key": {
      "configured": true,
      "masked": "AKIA****"
    },
    "secret_key": {
      "configured": true,
      "masked": "******"
    }
  },
  "last_checked_at": "2026-04-21T11:25:00+08:00"
}
```

### 4.4.3 `POST /api/v1/sources`
- **状态**：`Stable`
- **权限**：仅 `admin`
- **用途**：创建新的存储源

通用请求体：

```json
{
  "name": "媒体仓库",
  "driver_type": "s3",
  "is_enabled": true,
  "is_webdav_exposed": false,
  "webdav_read_only": true,
  "root_path": "/",
  "sort_order": 10,
  "config": {
    "endpoint": "https://s3.example.com",
    "region": "us-east-1",
    "bucket": "media",
    "force_path_style": true
  },
  "secret_patch": {
    "access_key": "AKIA...",
    "secret_key": "secret..."
  }
}
```

说明：
- `driver_type` 支持 `local | s3 | onedrive`
- 创建时建议自动验证一次连接

成功响应 `data`：

```json
{
  "source": {
    "id": 2,
    "name": "媒体仓库",
    "driver_type": "s3",
    "status": "online",
    "is_enabled": true,
    "is_webdav_exposed": false,
    "webdav_read_only": true,
    "webdav_slug": "media",
    "root_path": "/",
    "used_bytes": null,
    "total_bytes": null,
    "created_at": "2026-04-21T11:30:45+08:00",
    "updated_at": "2026-04-21T11:30:45+08:00"
  }
}
```

HTTP 状态：`201 Created`

错误码：
- `VALIDATION_ERROR`
- `SOURCE_DRIVER_UNSUPPORTED`
- `SOURCE_CONNECTION_FAILED`
- `CONFIG_INVALID`
- `SOURCE_NAME_CONFLICT`
- `ROLE_FORBIDDEN`

### 4.4.4 `PUT /api/v1/sources/:id`
- **状态**：`Stable`
- **权限**：仅 `admin`
- **用途**：更新存储源配置

请求体：

```json
{
  "name": "媒体仓库-新",
  "is_enabled": true,
  "is_webdav_exposed": true,
  "webdav_read_only": false,
  "root_path": "/movies",
  "sort_order": 20,
  "config": {
    "endpoint": "https://s3.example.com",
    "region": "us-east-1",
    "bucket": "media",
    "force_path_style": true
  },
  "secret_patch": {
    "access_key": null,
    "secret_key": "new-secret"
  }
}
```

`secret_patch` 规则：
- 字段缺失：保持不变
- 字段为字符串：更新
- 字段为 `null`：清空

成功响应 `data` 返回更新后的 `StorageSource`。

说明：
- `driver_type` 创建后不可变

### 4.4.5 `DELETE /api/v1/sources/:id`
- **状态**：`Stable`
- **权限**：仅 `admin`
- **用途**：删除存储源挂载配置

成功响应 `data`：

```json
{
  "deleted": true,
  "id": 2
}
```

删除前可阻止的场景：
- 是默认存储源
- 有活动上传任务
- 有活动下载任务
- 有未完成后台任务

错误码：
- `SOURCE_NOT_FOUND`
- `SOURCE_IN_USE`
- `ROLE_FORBIDDEN`

### 4.4.6 `POST /api/v1/sources/test`
- **状态**：`Stable`
- **权限**：仅 `admin`
- **用途**：测试尚未保存的存储源配置

请求体结构与 `POST /api/v1/sources` 基本一致。

成功响应 `data`：

```json
{
  "reachable": true,
  "status": "online",
  "latency_ms": 132,
  "checked_at": "2026-04-21T11:45:45+08:00",
  "warnings": []
}
```

说明：
- 测试成功不代表自动保存

### 4.4.7 `POST /api/v1/sources/:id/test`
- **状态**：`Stable`
- **权限**：仅 `admin`
- **用途**：重新测试已保存的存储源

成功响应 `data` 与 `/sources/test` 一致。

---
## 4.5 离线下载任务

### 4.5.0 统一约束
- P0 `/tasks` 仅表示离线下载任务
- `type` 在 P0 固定为 `download`
- Stable 默认采用轮询更新，不强制 WebSocket

### 4.5.1 `GET /api/v1/tasks`
- **状态**：`Stable`
- **用途**：获取当前用户可见的离线下载任务列表

Query 参数：
```http
GET /api/v1/tasks?status=running&page=1&page_size=20
```

可选字段：
- `status`
- `page`
- `page_size`
- `source_id`
- `keyword`

成功响应 `data`：

```json
{
  "items": [
    {
      "id": 1001,
      "type": "download",
      "status": "running",
      "source_id": 1,
      "save_path": "/downloads",
      "display_name": "movie.mkv",
      "source_url": "magnet:?xt=urn:btih:...",
      "progress": 45.2,
      "downloaded_bytes": 1288490188,
      "total_bytes": 2852126720,
      "speed_bytes": 2411724,
      "eta_seconds": 720,
      "error_message": null,
      "created_at": "2026-04-21T11:30:45+08:00",
      "updated_at": "2026-04-21T11:35:12+08:00",
      "finished_at": null
    }
  ]
}
```

### 4.5.2 `POST /api/v1/tasks`
- **状态**：`Stable`
- **用途**：创建新的离线下载任务

请求体：

```json
{
  "type": "download",
  "url": "magnet:?xt=urn:btih:...",
  "source_id": 1,
  "save_path": "/downloads"
}
```

成功响应 `data`：

```json
{
  "task": {
    "id": 1001,
    "type": "download",
    "status": "pending",
    "source_id": 1,
    "save_path": "/downloads",
    "display_name": "movie.mkv",
    "source_url": "magnet:?xt=urn:btih:...",
    "progress": 0,
    "downloaded_bytes": 0,
    "total_bytes": null,
    "speed_bytes": 0,
    "eta_seconds": null,
    "error_message": null,
    "created_at": "2026-04-21T11:30:45+08:00",
    "updated_at": "2026-04-21T11:30:45+08:00",
    "finished_at": null
  }
}
```

HTTP 状态：`202 Accepted`

错误码：
- `VALIDATION_ERROR`
- `SOURCE_NOT_FOUND`
- `PATH_INVALID`
- `PERMISSION_DENIED`
- `DOWNLOADER_UNAVAILABLE`
- `TASK_CREATE_FAILED`
- `SOURCE_READ_ONLY`

### 4.5.3 `GET /api/v1/tasks/:id`
- **状态**：`Stable`
- **用途**：获取单个任务详情

成功响应 `data`：

```json
{
  "id": 1001,
  "type": "download",
  "status": "running",
  "source_id": 1,
  "save_path": "/downloads",
  "display_name": "movie.mkv",
  "source_url": "magnet:?xt=urn:btih:...",
  "progress": 45.2,
  "downloaded_bytes": 1288490188,
  "total_bytes": 2852126720,
  "speed_bytes": 2411724,
  "eta_seconds": 720,
  "error_message": null,
  "created_at": "2026-04-21T11:30:45+08:00",
  "updated_at": "2026-04-21T11:35:12+08:00",
  "finished_at": null,
  "result": {
    "file_path": null,
    "source_id": 1
  }
}
```

### 4.5.4 `DELETE /api/v1/tasks/:id`
- **状态**：`Stable`
- **用途**：取消任务

Query 参数：
```http
DELETE /api/v1/tasks/1001?delete_file=false
```

成功响应 `data`：

```json
{
  "id": 1001,
  "canceled": true,
  "delete_file": false
}
```

说明：
- `pending | running | paused` 可以取消
- 已 `failed` / `canceled` 的重复取消建议幂等返回 `200`

---

## 4.6 系统配置与版本

### 4.6.0 统一约束
- `GET /api/v1/system/version`：已登录用户可读
- `GET /api/v1/system/config` / `PUT /api/v1/system/config`：仅 `admin`
- 返回的是前端可见/可编辑配置视图，而非完整后端运行配置

### 4.6.1 `GET /api/v1/system/config`
- **状态**：`Stable`
- **用途**：读取设置页公共配置

成功响应 `data`：`SystemConfigPublic`

错误码：
- `ROLE_FORBIDDEN`
- `CONFIG_INVALID`
- `INTERNAL_ERROR`

### 4.6.2 `PUT /api/v1/system/config`
- **状态**：`Stable`
- **用途**：更新设置页公共配置

请求体：

```json
{
  "site_name": "云匣",
  "multi_user_enabled": true,
  "default_source_id": 1,
  "max_upload_size": 21474836480,
  "default_chunk_size": 5242880,
  "webdav_enabled": true,
  "webdav_prefix": "/dav",
  "theme": "system",
  "language": "zh-CN",
  "time_zone": "Asia/Shanghai"
}
```

成功响应 `data`：更新后的 `SystemConfigPublic`

关键规则：
- 首版采用“完整表单提交”，不做 JSON Patch
- `default_source_id` 不存在时返回 `SOURCE_NOT_FOUND`
- `default_source_id` 存在但不可用时返回 `CONFIG_INVALID`
- `webdav_prefix` 必须以 `/` 开头且非空

### 4.6.3 `GET /api/v1/system/version`
- **状态**：`Stable`
- **用途**：关于页与版本展示

成功响应 `data`：

```json
{
  "service": "yunxia",
  "version": "1.0.0",
  "commit": "abcdef1",
  "build_time": "2026-04-21T11:30:45+08:00",
  "go_version": "go1.24.0",
  "api_version": "v1"
}
```

说明：
- `commit` / `build_time` 允许为 `null`
- 不把“检查更新”能力混入此接口

---

## 5. P0 Stable WebDAV 契约

### 5.1 入口与前缀

默认入口：

```text
/dav
```

系统级配置项：
- `webdav_enabled`
- `webdav_prefix`

### 5.2 路径映射规则

每个被暴露的存储源映射到 WebDAV 根下的一级子路径：

- `/dav/local/`
- `/dav/media/`
- `/dav/onedrive/`

映射依据：
- `StorageSource.webdav_slug`
- 创建时生成，后续应尽量保持稳定

### 5.3 认证方式

仅支持 Basic Auth：

```http
Authorization: Basic base64(username:password)
```

规则：
- 使用系统账号认证
- 不支持 Bearer Token 访问 WebDAV
- 强制 HTTPS；HTTP 下返回 `403 Forbidden`

### 5.4 权限模型

WebDAV 最终权限判断为：
1. 系统开启 WebDAV
2. 存储源 `is_webdav_exposed = true`
3. 用户通过 Basic Auth
4. 存储源不是全局只读写禁用态
5. ACL 允许对应路径的对应动作

说明：
- `webdav_read_only` 是存储源级总开关
- ACL 是路径级细粒度规则

### 5.5 Stable 支持的方法

#### `PROPFIND`
- 用途：列目录、取资源属性
- 支持：Depth `0` / `1`

#### `GET`
- 用途：读取文件内容
- 必须支持：`Range: bytes=...`

#### `PUT`
- 用途：写入文件
- 前提：非只读 + ACL 允许

#### `DELETE`
- 用途：删除文件或目录

#### `MKCOL`
- 用途：创建目录

#### `MOVE`
- 用途：移动 / 重命名

#### `COPY`
- 用途：复制资源

### 5.6 不纳入 P0 Stable 的 WebDAV 能力
- LOCK / UNLOCK
- PROPPATCH
- WebDAV 版本控制扩展
- 深度递归性能承诺
- 客户端私有扩展兼容性

### 5.7 最小属性保障集

文件至少保证：
- collection 标识
- content length
- content type
- last modified
- ETag

目录至少保证：
- collection 标识
- last modified

### 5.8 WebDAV 错误处理

推荐状态码：
- `401 Unauthorized`：Basic Auth 缺失或无效
- `403 Forbidden`：非 HTTPS、只读写请求、明确拒绝
- `404 Not Found`：路径不存在、存储源未暴露、对无权访问路径的隐藏策略
- `405 Method Not Allowed`：不支持方法
- `409 Conflict`：目标冲突或父路径不存在
- `500 Internal Server Error`：服务端异常

安全策略：
- 对“用户无权访问某路径”，优先返回 `404` 而不是 `403`
- 对“只读模式下写请求”，明确返回 `403`

### 5.9 缓存与限流

- PROPFIND 目录列表缓存 TTL：`30s`
- 写操作成功后尽量失效相关目录缓存
- WebDAV 单独限流，参考：`50 req/s`

### 5.10 前端设置页相关字段

系统级字段：
- `webdav_enabled`
- `webdav_prefix`

存储源级字段：
- `is_webdav_exposed`
- `webdav_read_only`
- `webdav_slug`

前端挂载地址展示建议由前端自行拼接：

```text
<origin> + webdav_prefix + "/" + webdav_slug + "/"
```

---
## 6. P1 Draft / Reserved 契约

## 6.1 Draft：初始化向导补充说明

不新增接口；前端仅依赖：
- `GET /api/v1/setup/status`
- `POST /api/v1/setup/init`

补充规则：
- 若 `setup_required=false` 访问 `/setup`，前端直接跳转 `/login` 或 `/files`
- `setup/init` 成功后直接进入登录态

## 6.2 Draft：用户管理接口

### 6.2.1 `GET /api/v1/users`

Query：
```http
GET /api/v1/users?page=1&page_size=20&keyword=alice&status=active
```

成功响应 `data`：

```json
{
  "items": [
    {
      "id": 2,
      "username": "alice",
      "email": "alice@example.com",
      "role": "normal",
      "status": "active",
      "last_login_at": "2026-04-21T10:20:00+08:00",
      "created_at": "2026-04-15T09:00:00+08:00"
    }
  ]
}
```

### 6.2.2 `POST /api/v1/users`

请求体：

```json
{
  "username": "alice",
  "password": "strong-password-123",
  "email": "alice@example.com",
  "role": "normal"
}
```

### 6.2.3 `PUT /api/v1/users/:id`

请求体示例：

```json
{
  "email": "alice@example.com",
  "role": "normal",
  "status": "active"
}
```

### 6.2.4 `POST /api/v1/users/:id/reset-password`

```json
{
  "new_password": "new-strong-password-123"
}
```

### 6.2.5 `POST /api/v1/users/:id/revoke-tokens`

成功响应：

```json
{
  "id": 2,
  "revoked": true
}
```

## 6.3 Draft：ACL 管理接口

### 6.3.1 `GET /api/v1/acl/rules`
```http
GET /api/v1/acl/rules?source_id=1&path=/projects
```

成功响应 `data`：

```json
{
  "items": [
    {
      "id": 301,
      "source_id": 1,
      "path": "/projects",
      "subject_type": "user",
      "subject_id": 2,
      "effect": "allow",
      "priority": 100,
      "permissions": {
        "read": true,
        "write": true,
        "delete": false,
        "share": false
      },
      "inherit_to_children": true
    }
  ]
}
```

### 6.3.2 `POST /api/v1/acl/rules`
```json
{
  "source_id": 1,
  "path": "/projects",
  "subject_type": "user",
  "subject_id": 2,
  "effect": "allow",
  "priority": 100,
  "permissions": {
    "read": true,
    "write": true,
    "delete": false,
    "share": false
  },
  "inherit_to_children": true
}
```

### 6.3.3 `PUT /api/v1/acl/rules/:id`
### 6.3.4 `DELETE /api/v1/acl/rules/:id`

说明：
- 规则计算顺序与优先级以后端最终实现为准
- 前端可先按此资源模型做管理页

## 6.4 Draft：回收站接口

### 6.4.1 `GET /api/v1/trash`
```http
GET /api/v1/trash?source_id=1&page=1&page_size=50
```

成功响应 `data`：

```json
{
  "items": [
    {
      "id": 501,
      "source_id": 1,
      "original_path": "/docs/report.pdf",
      "trash_path": "/.trash/2026/04/report.pdf",
      "name": "report.pdf",
      "size": 2457600,
      "deleted_at": "2026-04-21T11:30:45+08:00",
      "expires_at": "2026-05-21T11:30:45+08:00"
    }
  ]
}
```

### 6.4.2 `POST /api/v1/trash/:id/restore`
```json
{
  "id": 501,
  "restored": true,
  "restored_path": "/docs/report.pdf"
}
```

### 6.4.3 `DELETE /api/v1/trash/:id`
### 6.4.4 `DELETE /api/v1/trash?source_id=1`

## 6.5 Draft：下载任务暂停 / 恢复

### 6.5.1 `POST /api/v1/tasks/:id/pause`

成功响应：

```json
{
  "id": 1001,
  "status": "paused"
}
```

### 6.5.2 `POST /api/v1/tasks/:id/resume`

成功响应：

```json
{
  "id": 1001,
  "status": "running"
}
```

## 6.6 Draft：系统统计接口

### 6.6.1 `GET /api/v1/system/stats`

建议响应 `data`：

```json
{
  "sources_total": 3,
  "files_total": 12843,
  "downloads_running": 2,
  "downloads_completed": 15,
  "users_total": 1,
  "storage_used_bytes": 5368709120
}
```

## 6.7 Draft：分享相关接口（文件 / 目录分享）

资源：`ShareLink`

当前 Draft 范围：
- 支持**文件分享**与**目录分享**
- 创建分享时要求当前用户对目标路径具备 `share` 权限
- 分享列表仅返回当前登录用户自己创建的分享

### 6.7.1 `GET /api/v1/shares`

成功响应 `data`：

```json
{
  "items": [
    {
      "id": 901,
      "source_id": 1,
      "path": "/docs/report.pdf",
      "name": "report.pdf",
      "is_dir": false,
      "link": "/s/4c7c1d0d-6d1a-4cc2-8c67-2d53d83d8054",
      "has_password": false,
      "expires_at": "2026-04-21T12:30:45+08:00",
      "created_at": "2026-04-21T11:30:45+08:00"
    }
  ]
}
```

### 6.7.2 `POST /api/v1/shares`

请求体：

```json
{
  "source_id": 1,
  "path": "/docs/report.pdf",
  "expires_in": 3600,
  "password": "optional-password"
}
```

说明：
- `expires_in` 单位为秒；`<=0` 表示不过期
- `password` 可为空；若非空则为密码保护分享
- `path` 可以是文件路径，也可以是目录路径

### 6.7.3 `GET /api/v1/shares/:id`

成功响应：

```json
{
  "share": {
    "id": 901,
    "source_id": 1,
    "path": "/docs/report.pdf",
    "name": "report.pdf",
    "is_dir": false,
    "link": "/s/4c7c1d0d-6d1a-4cc2-8c67-2d53d83d8054",
    "has_password": false,
    "expires_at": "2026-04-21T12:30:45+08:00",
    "created_at": "2026-04-21T11:30:45+08:00"
  }
}
```

说明：
- 仅返回当前登录用户自己拥有的分享
- 非 owner 访问返回 `403 + PERMISSION_DENIED`

### 6.7.4 `PUT /api/v1/shares/:id`

请求体：

```json
{
  "expires_in": 7200,
  "password": "new-share-password"
}
```

说明：
- 字段均为可选；未传表示保持不变
- `expires_in > 0`：重新设置为“从当前时间起 N 秒后过期”
- `expires_in <= 0`：清空过期时间
- `password = ""`：清空访问密码
- `password = "non-empty"`：更新访问密码
- 当前不支持修改 `source_id`、`path`、`name`

### 6.7.5 `DELETE /api/v1/shares/:id`

成功响应：

```json
{
  "id": 901,
  "deleted": true
}
```

说明：
- 当前 `DELETE` 同时承担“提前失效 / 撤销分享”语义

### 6.7.6 `GET /s/:token`

当前 Draft 语义：
- 文件分享：`302` 跳转到后端受控下载地址
- 目录分享：
  - `GET /s/:token`：返回分享根目录列表
  - `GET /s/:token?path=/subdir`：返回子目录列表
  - `GET /s/:token?path=/subdir/file.ext`：若命中文件则 `302` 跳转到后端受控下载地址
- 密码保护分享：
  - 未提供密码：`401 + SHARE_PASSWORD_REQUIRED`
  - 密码错误：`401 + SHARE_PASSWORD_INVALID`
- 已过期分享：`410 + SHARE_EXPIRED`

当前支持的查询参数：
- `password`：可选，访问密码
- `path`：可选；仅目录分享使用，表示**相对于分享根**的目标路径，必须以 `/` 开头，且不允许 `..`
- `page`：可选；仅目录分享列表使用，默认 `1`
- `page_size`：可选；仅目录分享列表使用，默认 `200`
- `sort_by`：可选；当前支持 `name / size / modified_at`，默认 `name`
- `sort_order`：可选；当前支持 `asc / desc`，默认 `asc`
- `disposition`：可选，默认 `attachment`

目录分享成功响应 `data`：

```json
{
  "share": {
    "id": 901,
    "source_id": 1,
    "path": "/albums",
    "name": "albums",
    "is_dir": true,
    "link": "/s/4c7c1d0d-6d1a-4cc2-8c67-2d53d83d8054",
    "has_password": false,
    "expires_at": "2026-04-21T12:30:45+08:00",
    "created_at": "2026-04-21T11:30:45+08:00"
  },
  "current_path": "/2026",
  "current_dir": {
    "name": "2026",
    "path": "/2026",
    "parent_path": "/",
    "is_root": false
  },
  "breadcrumbs": [
    {
      "name": "albums",
      "path": "/"
    },
    {
      "name": "2026",
      "path": "/2026"
    }
  ],
  "pagination": {
    "page": 1,
    "page_size": 50,
    "total": 1,
    "total_pages": 1
  },
  "items": [
    {
      "name": "photo.txt",
      "path": "/2026/photo.txt",
      "parent_path": "/2026",
      "is_dir": false,
      "preview_type": "text",
      "size": 12,
      "mime_type": "text/plain; charset=utf-8",
      "extension": ".txt",
      "modified_at": "2026-04-21T12:20:00+08:00",
      "created_at": "2026-04-21T12:20:00+08:00",
      "can_preview": true,
      "can_download": true,
      "thumbnail_url": null
    }
  ]
}
```

目录分享额外错误语义：
- 越界或非法 `path`：`400 + PATH_INVALID`
- 目录内目标不存在：`404 + FILE_NOT_FOUND`

目录分享附加约定：
- `items` 始终表示当前页数据
- `breadcrumbs` 已按“分享根 -> 当前目录”展开，前端可直接渲染面包屑
- `current_dir` 可直接作为目录页标题区数据
- `pagination` 可直接用于页码器渲染
- `preview_type` 当前返回值范围：
  - `directory`
  - `image`
  - `video`
  - `audio`
  - `text`
  - `pdf`
  - `json`
  - `binary`

## 6.8 Reserved：全文搜索 / 索引任务

资源：`SearchIndexJob`

保留路由方向：
- `POST /api/v1/search/index/rebuild`
- `GET /api/v1/search/index/status`

## 6.9 Reserved：审计 / 日志接口

资源：`AuditLogEntry`

保留路由方向：
- `GET /api/v1/audit/logs`

## 6.10 Draft / Reserved 使用规则

### Draft
- 可用于页面开发、Mock、状态流设计
- 联调前必须重新对照最新文档版本

### Reserved
- 仅可用于路由命名参考与信息架构占位
- 不可作为真实字段契约依赖

---
## 7. Mock、联调与变更管理规则

### 7.1 Mock 优先级

#### 第一优先级：必须优先提供 Mock 的 Stable 接口
- `GET /api/v1/setup/status`
- `POST /api/v1/setup/init`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/me`
- `GET /api/v1/files`
- `GET /api/v1/files/search`
- `POST /api/v1/files/mkdir`
- `POST /api/v1/files/rename`
- `POST /api/v1/files/move`
- `POST /api/v1/files/copy`
- `DELETE /api/v1/files`
- `POST /api/v1/files/access-url`
- `POST /api/v1/upload/init`
- `PUT /api/v1/upload/chunk`
- `POST /api/v1/upload/finish`
- `GET /api/v1/upload/sessions`
- `DELETE /api/v1/upload/sessions/:upload_id`
- `GET /api/v1/sources?view=navigation`
- `GET /api/v1/sources?view=admin`
- `GET /api/v1/sources/:id`
- `POST /api/v1/sources/test`
- `POST /api/v1/sources`
- `PUT /api/v1/sources/:id`
- `DELETE /api/v1/sources/:id`
- `POST /api/v1/sources/:id/test`
- `GET /api/v1/tasks`
- `POST /api/v1/tasks`
- `GET /api/v1/tasks/:id`
- `DELETE /api/v1/tasks/:id`
- `GET /api/v1/system/config`
- `PUT /api/v1/system/config`
- `GET /api/v1/system/version`

#### 第二优先级：Draft Mock
- `GET /api/v1/users`
- `POST /api/v1/users`
- `PUT /api/v1/users/:id`
- `POST /api/v1/users/:id/reset-password`
- `POST /api/v1/users/:id/revoke-tokens`
- `GET /api/v1/acl/rules`
- `POST /api/v1/acl/rules`
- `PUT /api/v1/acl/rules/:id`
- `DELETE /api/v1/acl/rules/:id`
- `GET /api/v1/trash`
- `POST /api/v1/trash/:id/restore`
- `DELETE /api/v1/trash/:id`
- `DELETE /api/v1/trash`
- `POST /api/v1/tasks/:id/pause`
- `POST /api/v1/tasks/:id/resume`
- `GET /api/v1/system/stats`
- `GET /api/v1/shares`
- `GET /api/v1/shares/:id`
- `POST /api/v1/shares`
- `PUT /api/v1/shares/:id`
- `DELETE /api/v1/shares/:id`
- `GET /s/:token`

#### 第三优先级：Reserved 仅占位
- `SearchIndexJob`
- `AuditLogEntry`

### 7.2 可先独立推进的前端页面

可立即推进：
- `/login`
- `/setup`
- `/files`
- `/downloads`
- `/sources`
- `/settings`
- 文件预览抽屉
- 上传面板

可先做壳子、但需标注 Draft 风险：
- 用户管理
- ACL 管理
- 回收站
- 系统统计概览
- 分享管理（文件 + 目录浏览 Draft）

不建议现在深做：
- 全文搜索管理
- 审计日志高级检索

### 7.3 Stable 联调冻结规则

一旦开始真实联调，Stable 接口默认冻结：
- 方法
- 路径
- 参数名
- JSON 字段名
- 字段类型
- 基本响应结构
- 错误码名称

允许的变更：
- 新增可选字段
- 补充 `message`
- 补充不影响前端的 `meta`
- 向后兼容地扩展枚举（需说明）

禁止的变更：
- 改字段名
- 改字段类型
- 删字段
- 改响应包裹层
- 改 JSON 命名风格
- 无通知地改变错误码语义

### 7.4 Draft 联调规则

- 变更必须留记录
- 尽量保持资源模型稳定
- Draft 转 Stable 前，需要一次前后端共同收口

### 7.5 Mock 数据规则

Mock 数据必须满足真实约束：
- 路径以 `/` 开头
- 时间用 RFC3339
- 文件大小为 bytes 整数
- 枚举值与文档一致
- 错误码必须来自错误码总表

每个关键页面至少覆盖：
1. 成功场景
2. 空状态
3. 异常场景

### 7.6 错误处理联调规则

前端处理顺序：
1. HTTP status
2. `body.code`
3. `body.message`

关键处理约定：
- `401 + AUTH_TOKEN_EXPIRED` → 尝试 refresh
- `401 + AUTH_TOKEN_INVALID` → 直接回登录页
- `403 + PERMISSION_DENIED` → 显示无权限
- `429 + RATE_LIMITED` → 显示稍后重试
- `503 + DOWNLOADER_UNAVAILABLE` → 下载页提示依赖服务不可用

### 7.7 文件预览与下载联调规则

预览与下载优先通过：
- `POST /api/v1/files/access-url`

前端不应自行推断：
- 本地 / S3 / OneDrive 的底层访问方式
- Bearer Token 如何附着到原生媒体标签

### 7.8 上传联调规则

必须接受的现实语义：
- 后端可恢复上传会话状态
- 浏览器不一定能自动恢复本地文件句柄
- 刷新后必要时提示用户重新选择同一文件

### 7.9 变更记录规则

建议内置简单 changelog，每次修改至少记录：
- 日期
- 变更接口
- 变更级别：`Stable / Draft / Reserved`
- 是否破坏兼容
- 前端是否需跟进

### 7.10 API 真相源规则

前后端联调阶段：

> **接口真相源以本 API 契约文档为准。**

---

## 8. Changelog

### 2026-04-21
- Added `GET /api/v1/setup/status` as Stable
- Added `POST /api/v1/setup/init` as Stable
- Added `POST /api/v1/files/rename` as Stable
- Added `POST /api/v1/files/access-url` as Stable
- Added `GET /api/v1/upload/sessions` as Stable
- Added `DELETE /api/v1/upload/sessions/:upload_id` as Stable
- Added `POST /api/v1/sources/test` as Stable
- Added `StorageSource.webdav_slug` as Stable field
- Classified system stats, user management, ACL management, trash management, task pause/resume as Draft
- Classified share, full-text indexing, audit logs as Reserved
- Promoted share APIs to Draft with file + directory-share semantics:
  - `GET /api/v1/shares`
  - `GET /api/v1/shares/:id`
  - `POST /api/v1/shares`
  - `PUT /api/v1/shares/:id`
  - `DELETE /api/v1/shares/:id`
  - `GET /s/:token`
