# Yunxia Backend API Contract

> 更新时间：2026-04-23  
> 对应实现：`backend/` 当前 `feature/global-permission-refactor` 版本  
> 校验依据：
> - 本地 `go test ./...` 通过
> - 测试机 `/home/hjx/Yunxia-verify-global-permission/backend` 执行 `go test ./... -count=1` 通过
> - 测试机 live smoke 已覆盖 `setup/auth/system/users/acl/sources/files/upload/trash/tasks/shares/webdav` 主链路
> - S3 相关接口由后端集成测试覆盖（live smoke 以本地源为主）

## 1. 通用约定

- Base URL：`/api/v1`
- 响应包络统一为：
  - `success: boolean`
  - `code: string`
  - `message: string`
  - `data: object | null`
  - `meta.request_id: string`
  - `meta.timestamp: string`
- 认证方式：
  - 普通接口：`Authorization: Bearer <access_token>`
  - 文件下载临时地址：`access_token` 查询参数
  - WebDAV：Basic Auth（用户名/密码）
- 时间字段：RFC3339
- 分页查询：当前接口普遍使用 `page` / `page_size`

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

### 2.2 capability 列表

`/auth/me` 返回当前用户 capability 集合。当前内建 capability：

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
- cross-user
  - `task.read_all`
  - `task.manage_all`
  - `share.read_all`
  - `share.manage_all`

### 2.3 当前角色语义

| role_key | 说明 |
|---|---|
| `super_admin` | 拥有全部 capability；初始化首用户固定为该角色；保留 runtime ACL bypass |
| `admin` | 拥有治理 capability，但没有 `source.secret.read`；只能管理 `operator/user` |
| `operator` | 只读统计、源读取/测试、跨用户任务/分享治理 |
| `user` | 无治理 capability；主要依赖 ACL 访问数据面 |

### 2.4 当前关键规则

- 系统初始化首用户固定创建为 `super_admin`
- 禁止移除最后一个激活的 `super_admin`
- `GET /api/v1/sources?view=navigation`：仅要求已登录；普通用户结果会按 ACL 过滤
- `view=admin` / source 详情 / source 增删改测：按 capability 控制
- `task` / `share`：默认 owner 可管理；具备跨用户 capability 的角色可管理所有人数据
- S3 明文 secret 仅 `source.secret.read` 可见；当前仅 `super_admin` 可见

## 3. 路由总览

### 3.1 初始化与认证

| 方法 | 路径 | 鉴权 / 权限 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/setup/status` | 无 | - | 200，`{is_initialized, setup_required, has_super_admin}` |
| POST | `/setup/init` | 无，仅未初始化可调用 | `username,password,email` | 201，`{user,tokens}` |
| POST | `/auth/login` | 无 | `username,password` | 200，`{user,tokens}` |
| POST | `/auth/refresh` | 无 | `refresh_token` | 200，`{tokens}` |
| POST | `/auth/logout` | Bearer | `refresh_token` | 200，空数据 |
| GET | `/auth/me` | Bearer | - | 200，`{user,capabilities[]}` |

补充：

- `POST /auth/refresh` 失败返回 `401 AUTH_REFRESH_TOKEN_INVALID`
- `POST /auth/logout` 需要 Bearer + `refresh_token`，用于撤销 refresh token

### 3.2 system

| 方法 | 路径 | 权限 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/health` | 无 | - | 200，服务健康信息 |
| GET | `/system/version` | 已登录 | - | 200，`{service,version,commit,build_time,go_version,api_version}` |
| GET | `/system/stats` | `system.stats.read` | - | 200，系统聚合统计 |
| GET | `/system/config` | `system.config.read` | - | 200，系统配置 |
| PUT | `/system/config` | `system.config.write` | `site_name,multi_user_enabled,default_source_id,max_upload_size,default_chunk_size,webdav_enabled,webdav_prefix,theme,language,time_zone` | 200，更新后的系统配置 |

### 3.3 users

| 方法 | 路径 | 权限 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/users` | `user.read` | query: `page,page_size,keyword,status` | 200，`{items[]}` |
| POST | `/users` | `user.create` + `user.role.assign` | `username,password,email,role_key` | 201，`{user}` |
| PUT | `/users/:id` | `user.update` + `user.role.assign` + `user.lock` | `email,role_key,status` | 200，`{user}` |
| POST | `/users/:id/reset-password` | `user.password.reset` | `new_password` | 200，空数据 |
| POST | `/users/:id/revoke-tokens` | `user.tokens.revoke` | - | 200，`{id,revoked}` |

补充：

- `admin` 只能创建/更新 `operator`、`user`
- 撤销令牌后，旧 access token 将返回 `401 AUTH_TOKEN_INVALID`

### 3.4 ACL

| 方法 | 路径 | 权限 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/acl/rules` | `acl.read` | query: `source_id,path` | 200，`{items[]}` |
| POST | `/acl/rules` | `acl.manage` | `source_id,path,subject_type,subject_id,effect,priority,permissions,inherit_to_children` | 201，`{rule}` |
| PUT | `/acl/rules/:id` | `acl.manage` | `path,subject_type,subject_id,effect,priority,permissions,inherit_to_children` | 200，`{rule}` |
| DELETE | `/acl/rules/:id` | `acl.manage` | - | 200，`{deleted,id}` |

`permissions` 结构：

```json
{
  "read": true,
  "write": false,
  "delete": false,
  "share": false
}
```

### 3.5 sources

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

- 通用：`name,driver_type,is_enabled,is_webdav_exposed,webdav_read_only,root_path,sort_order`
- local：`config.base_path`
- s3：`config.endpoint,region,bucket,base_prefix,force_path_style` + `secret_patch.access_key/secret_key`

补充：

- 初始化完成后会自动创建默认本地源：`本地存储`
- local 源会自动生成/保留 `webdav_slug`
- `GET /sources/:id` 对 S3 源会返回 `secret_fields`；只有 `super_admin` 可见明文 config secret

### 3.6 files

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
| GET | `/files/download` | Bearer 或 `access_token` | query: `source_id,path,disposition[,access_token]` | local: 200 文件流；S3: 302 跳转 presigned URL |

补充：

- `delete_mode` 留空时默认按 `trash` 处理
- 数据面接口会做 ACL 校验；失败返回 `403 ACL_DENIED`
- S3 下载 / 访问地址为重定向语义；本地源返回应用内签名下载链接

### 3.7 upload

| 方法 | 路径 | 鉴权 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| POST | `/upload/init` | Bearer | `source_id,path,filename,file_size,file_hash,last_modified_at` | 200，`UploadInitResponse` |
| PUT | `/upload/chunk` | Bearer | query: `upload_id,index`，body 为二进制分片 | 200，`{upload_id,index,received_bytes,already_uploaded}` |
| POST | `/upload/finish` | Bearer | `upload_id[,parts[]]` | 201，`{completed,upload_id,file}` |
| GET | `/upload/sessions` | Bearer | query: `source_id` | 200，`{items[]}` |
| DELETE | `/upload/sessions/:upload_id` | Bearer | path: `upload_id` | 200，空数据 |

补充：

- 本地源返回 `transport.mode=server_chunk`
- S3 源返回 multipart 直传说明 `part_instructions[]`

### 3.8 trash

| 方法 | 路径 | 鉴权 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/trash` | Bearer | query: `source_id,page,page_size` | 200，`{items[]}` |
| POST | `/trash/:id/restore` | Bearer | path: `id` | 200，`{id,restored,restored_path}` |
| DELETE | `/trash/:id` | Bearer | path: `id` | 200，`{id,deleted}` |
| DELETE | `/trash` | Bearer | query: `source_id` | 200，`{source_id,cleared,deleted_count}` |

### 3.9 tasks

| 方法 | 路径 | 鉴权 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/tasks` | Bearer | - | 200，`{items[]}` |
| POST | `/tasks` | Bearer | `type,url,source_id,save_path` | 202，`{task}` |
| GET | `/tasks/:id` | Bearer | path: `id` | 200，`{task}` |
| POST | `/tasks/:id/pause` | Bearer | path: `id` | 200，`{id,status}` |
| POST | `/tasks/:id/resume` | Bearer | path: `id` | 200，`{id,status}` |
| DELETE | `/tasks/:id` | Bearer | query: `delete_file` | 200，`{id,canceled,delete_file}` |

补充：

- 普通用户默认仅能看到/操作自己的任务
- 具备 `task.read_all` / `task.manage_all` capability 的角色可跨用户治理
- 保存路径同样受 ACL 约束

### 3.10 shares

| 方法 | 路径 | 鉴权 | 主要输入 | 成功返回 |
|---|---|---|---|---|
| GET | `/shares` | Bearer | - | 200，`{items[]}` |
| GET | `/shares/:id` | Bearer | path: `id` | 200，`{share}` |
| POST | `/shares` | Bearer | `source_id,path,expires_in,password` | 201，`{share}` |
| PUT | `/shares/:id` | Bearer | `expires_in,password` | 200，`{share}` |
| DELETE | `/shares/:id` | Bearer | path: `id` | 200，`{id,deleted}` |
| GET | `/s/:token` | 无 | query: `password,path,page,page_size` | 文件：302 到下载地址；目录：200 目录浏览；异常：401/404 |

补充：

- 普通用户默认仅能管理自己的分享
- 具备 `share.read_all` / `share.manage_all` capability 的角色可跨用户治理
- 密码保护分享未带密码时返回 `401 SHARE_PASSWORD_REQUIRED`

### 3.11 WebDAV

可用方法：

- `OPTIONS`
- `HEAD`
- `GET`
- `PUT`
- `DELETE`
- `PROPFIND`
- `MKCOL`
- `COPY`
- `MOVE`

路由形态：

- `/dav/:slug`
- `/dav/:slug/*filepath`

约束：

- 使用 Basic Auth
- 仅对 `is_webdav_exposed=true` 的源可访问
- 需要 HTTPS 语义；反向代理场景应传 `X-Forwarded-Proto: https`
- 普通用户仍受 ACL 约束

## 4. 关键返回结构

### 4.1 UserSummary

```json
{
  "id": 1,
  "username": "admin",
  "email": "admin@example.com",
  "role_key": "super_admin",
  "status": "active",
  "created_at": "2026-04-23T15:00:00+08:00"
}
```

### 4.2 TokenPair

```json
{
  "access_token": "...",
  "refresh_token": "...",
  "expires_in": 900,
  "refresh_expires_in": 604800,
  "token_type": "Bearer"
}
```

### 4.3 StorageSourceView

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
  "used_bytes": 0,
  "total_bytes": null,
  "created_at": "2026-04-23T15:00:00+08:00",
  "updated_at": "2026-04-23T15:00:00+08:00"
}
```

### 4.4 FileItem

```json
{
  "name": "greeting.txt",
  "path": "/archive/greeting.txt",
  "parent_path": "/archive",
  "source_id": 1,
  "is_dir": false,
  "size": 12,
  "mime_type": "text/plain; charset=utf-8",
  "extension": ".txt",
  "etag": "...",
  "modified_at": "2026-04-23T15:00:00+08:00",
  "created_at": "2026-04-23T15:00:00+08:00",
  "can_preview": true,
  "can_download": true,
  "can_delete": true,
  "thumbnail_url": null
}
```

## 5. 关键错误码

### 5.1 auth / permission

- `AUTH_TOKEN_MISSING`
- `AUTH_TOKEN_INVALID`
- `AUTH_TOKEN_EXPIRED`
- `AUTH_ACCOUNT_LOCKED`
- `AUTH_INVALID_CREDENTIALS`
- `AUTH_REFRESH_TOKEN_INVALID`
- `CAPABILITY_DENIED`
- `ACL_DENIED`
- `PERMISSION_DENIED`
- `ROLE_ASSIGNMENT_FORBIDDEN`

### 5.2 source / config

- `SOURCE_NOT_FOUND`
- `SOURCE_DRIVER_UNSUPPORTED`
- `SOURCE_CONNECTION_FAILED`
- `SOURCE_NAME_CONFLICT`
- `SOURCE_IN_USE`
- `CONFIG_INVALID`
- `PATH_INVALID`

### 5.3 file / upload / trash

- `FILE_NOT_FOUND`
- `FILE_ALREADY_EXISTS`
- `FILE_NAME_INVALID`
- `FILE_MOVE_CONFLICT`
- `FILE_COPY_CONFLICT`
- `FILE_IS_DIRECTORY`
- `UPLOAD_SESSION_NOT_FOUND`
- `UPLOAD_ALREADY_FINISHED`
- `TRASH_ITEM_NOT_FOUND`

### 5.4 share / task

- `SHARE_NOT_FOUND`
- `SHARE_PASSWORD_REQUIRED`
- `TASK_NOT_FOUND`
- `TASK_SOURCE_NOT_FOUND`

## 6. 当前文档特别说明

1. `GET /api/v1/system/version` 不是公开接口，必须登录。
2. `GET /api/v1/files/download` 是公开路由，但必须携带 Bearer 或签名 `access_token`。
3. `GET /api/v1/sources?view=navigation` 只要求登录，不要求 `source.read` capability。
4. live smoke 已在测试机验证：
   - 初始化/登录/刷新/登出
   - 系统配置与统计
   - 用户创建、更新、重置密码、撤销令牌
   - ACL 创建/更新/删除
   - 本地源 CRUD 与 retest
   - 文件/上传/搜索/下载/回收站
   - 离线任务主链路
   - 分享公开访问
   - WebDAV 基本读写
5. S3 相关：
   - `/files/download` 对 S3 返回 302 到 presigned URL
   - `/files/search` 已有显式集成测试覆盖
   - 线上 live smoke 未接入真实 S3 凭据时，以集成测试结果为准
