# Yunxia Backend Changelog

> 说明：当前后端仍处于快速迭代期，先按“里程碑 + 能力范围”记录整体变更，而不是按正式版本号切分。

## 当前快照

- 后端根目录：`backend/`
- 技术栈：Go 1.25、Gin、GORM、SQLite、JWT、bcrypt、Aria2、AWS SDK for Go v2
- 当前状态：已完成全局权限模型重构；本地存储主链路、S3 MVP、离线下载、分享/ACL/上传链路已全部接入新权限模型
- 当前验证：`cd backend && go test ./...` 已通过

---

## 2026-04-23

### 1. 全局权限模型重构

- 彻底移除旧的 `admin / user(normal)` + `is_locked` 判权模型
- 用户实体、仓储、JWT、请求上下文统一切换为：
  - `role_key`
  - `status`
  - `capabilities`
- 新增权限真相源：
  - `internal/domain/permission/roles.go`
  - `internal/domain/permission/status.go`
  - `internal/domain/permission/capabilities.go`
  - `internal/domain/permission/resolver.go`
  - `internal/domain/permission/checker.go`

当前固定角色：

- `super_admin`
- `admin`
- `operator`
- `user`

当前固定状态：

- `active`
- `locked`

### 2. 认证 / 初始化接口变更

- 初始化首个用户固定为 `super_admin`
- `GET /api/v1/setup/status` 返回字段改为：
  - `has_super_admin`
- `GET /api/v1/auth/me` 返回结构升级为：
  - `user.role_key`
  - `user.status`
  - `capabilities[]`
- access / refresh token 内部 claim 从 `role` 切换为 `role_key`

### 3. 治理面接口改为 capability 判权

- 移除 `RequireAdmin()`
- 新增 `RequireCapability(...)`
- 当前治理接口判权方式：
  - `system.*`
  - `user.*`
  - `acl.*`
  - `source.*`

已落地的关键变化：

- `GET /api/v1/system/stats` → `system.stats.read`
- `GET /api/v1/system/config` → `system.config.read`
- `PUT /api/v1/system/config` → `system.config.write`
- 用户管理接口拆分到 `user.read / create / update / lock / password.reset / tokens.revoke / role.assign`
- ACL 管理接口拆分到 `acl.read / acl.manage`
- source 接口拆分到 `source.read / test / create / update / delete`

### 4. 用户管理硬规则收口

- `admin` 只能创建 / 更新 `operator`、`user`
- `admin` 不能创建 / 提升 `admin` / `super_admin`
- 系统始终至少保留 1 个 `active super_admin`
- 用户管理对外字段统一为：
  - `role_key`
  - `status`

新增错误：

- `ROLE_ASSIGNMENT_FORBIDDEN`
- `LAST_SUPER_ADMIN_FORBIDDEN`

### 5. source secret 可见性收口

- `GET /api/v1/sources/:id` 默认仅返回公开配置 + `secret_fields`
- 只有具备 `source.secret.read` 的请求才会看到明文：
  - `access_key`
  - `secret_key`
- 当前仅 `super_admin` 拥有该能力

### 6. 数据面权限行为更新

- task/share 改为 **owner-or-capability**
  - `task.read_all`
  - `task.manage_all`
  - `share.read_all`
  - `share.manage_all`
- upload 会话操作严格限定 owner
- `admin / operator / user` 的 runtime ACL 字符串 bypass 已移除
- `super_admin` 当前保留 runtime ACL bypass，便于系统级维护与初始化

### 7. 测试与验证

- 新增/更新：
  - capability resolver 单测
  - user repo / JWT 新模型单测
  - setup/auth 当前用户能力测试
  - source secret 可见性测试
  - governance capability HTTP 集成测试
  - 用户角色边界与最后 super admin 保护测试
- 当前验证结果：
  - `go test ./...` 通过
  - 旧权限关键字扫描通过：
    - `RequireAdmin(`
    - `IsLocked`
    - `is_locked`
    - 旧 `admin/normal` 判权逻辑

---

## 2026-04-21

### 1. 后端工程初始化

- 新建后端根目录 `backend/`
- 完成基础目录结构：
  - `backend/cmd/server`
  - `backend/internal/application`
  - `backend/internal/domain`
  - `backend/internal/infrastructure`
  - `backend/internal/interfaces`
- 建立服务启动入口：
  - `backend/cmd/server/main.go`
- 完成基础依赖接线：
  - Gin 路由
  - SQLite / GORM
  - 安全组件（JWT / bcrypt）
  - 仓储层与服务层装配

### 2. 认证、初始化、系统配置

- 完成首个管理员初始化流程
- 完成登录、刷新令牌、登出、当前用户信息接口
- 完成系统配置读取与更新
- 完成健康检查与版本信息接口

已落地的核心能力：

- `GET /api/v1/setup/status`
- `POST /api/v1/setup/init`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/refresh`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/me`
- `GET /api/v1/health`
- `GET /api/v1/system/version`
- `GET /api/v1/system/config`
- `PUT /api/v1/system/config`

### 3. 存储源管理（local first）

- 完成本地存储源的创建、更新、删除、测试、详情、列表
- 完成默认本地存储源自动初始化
- 支持 `view=navigation` 与 `view=admin` 两类存储源列表视图

已落地的核心能力：

- `GET /api/v1/sources`
- `GET /api/v1/sources/:id`
- `POST /api/v1/sources`
- `PUT /api/v1/sources/:id`
- `DELETE /api/v1/sources/:id`
- `POST /api/v1/sources/test`
- `POST /api/v1/sources/:id/test`

### 4. 本地文件管理主链路

- 完成 local source 下的文件浏览、搜索、目录创建、重命名、移动、复制、删除
- 完成短时访问地址生成
- 完成基于访问令牌或 Bearer token 的下载访问

已落地的核心能力：

- `GET /api/v1/files`
- `GET /api/v1/files/search`
- `POST /api/v1/files/mkdir`
- `POST /api/v1/files/rename`
- `POST /api/v1/files/move`
- `POST /api/v1/files/copy`
- `DELETE /api/v1/files`
- `POST /api/v1/files/access-url`
- `GET /api/v1/files/download`

### 5. 本地上传主链路

- 完成 local source 下的上传初始化
- 完成后端接收 chunk 的 `server_chunk` 模式
- 完成上传完成合并、活动上传会话查询、上传取消
- 支持秒传命中与未完成会话恢复

已落地的核心能力：

- `POST /api/v1/upload/init`
- `PUT /api/v1/upload/chunk`
- `POST /api/v1/upload/finish`
- `GET /api/v1/upload/sessions`
- `DELETE /api/v1/upload/sessions/:upload_id`

### 6. WebDAV（local）

- 完成本地存储源的 WebDAV 暴露能力
- 支持 Basic Auth
- 支持只读/非只读配置
- 支持 HTTPS 前缀与系统配置联动

说明：

- 当前 WebDAV 仅支持 `local` driver
- S3 WebDAV 不在当前 MVP 范围内

### 7. 离线下载与 Aria2 集成

- 完成离线任务创建、详情、列表、删除
- 接入 `TaskService -> Downloader -> Aria2Client` 分层
- 完成任务状态同步
- 完成任务暂停/恢复能力

已落地的核心能力：

- `POST /api/v1/tasks`
- `GET /api/v1/tasks`
- `GET /api/v1/tasks/:id`
- `DELETE /api/v1/tasks/:id`
- `POST /api/v1/tasks/:id/pause`
- `POST /api/v1/tasks/:id/resume`

相关增强：

- 增加 downloader 抽象的 `Pause` / `Resume`
- 增加 fake downloader 测试装配
- 保持 `cmd/server` 默认注入真实 `Aria2Client`

### 7.1 系统统计接口（Draft）

- 新增 `GET /api/v1/system/stats`
- 当前权限模型：
  - 仅 `admin`
- 当前统计口径：
  - `sources_total`：全部存储源数量
  - `users_total`：全部用户数量
  - `downloads_running`：状态为 `running` 的下载任务数量
  - `downloads_completed`：状态为 `completed` 的下载任务数量
  - `files_total`：启用中的存储源内可见文件总数
  - `storage_used_bytes`：启用中的存储源内可见文件大小总和
- 当前实现支持：
  - local source 递归统计
  - 已注册 file driver 的非 local source 递归统计
  - 自动忽略 `.trash` / `.system`

### 7.2 回收站接口（Draft）

- 新增回收站元数据表：
  - `trash_items`
- 删除到回收站时：
  - 真实文件/目录继续移动到 `/.trash/...`
  - 同步写入 `trash_items` 元数据
- 新增接口：
  - `GET /api/v1/trash`
  - `POST /api/v1/trash/:id/restore`
  - `DELETE /api/v1/trash/:id`
  - `DELETE /api/v1/trash?source_id=...`
- 当前实现语义：
  - list 以 `trash_items` 为真相源
  - restore 恢复到 `original_path`
  - restore 时若原路径已存在，返回冲突
  - delete one / clear source 会同时删除真实存储对象与元数据
  - local 与 S3 均已接入
- 当前保留规则：
  - `expires_at = deleted_at + 30 天`
  - list / files/search / files/list 会继续隐藏 `.trash` / `.system`

### 7.3 用户管理接口（Draft）

- 新增管理员用户管理接口：
  - `GET /api/v1/users`
  - `POST /api/v1/users`
  - `PUT /api/v1/users/:id`
  - `POST /api/v1/users/:id/reset-password`
  - `POST /api/v1/users/:id/revoke-tokens`
- 当前权限模型：
  - 全部接口仅 `admin`
- 当前实现约定：
  - 内部角色仍使用 `admin / user`
  - 对外接口角色映射为 `admin / normal`
  - 用户状态由 `IsLocked` 映射为 `active / locked`
  - `last_login_at` 当前返回 `null`
- 当前回收 token 语义：
  - `revoke-tokens` 通过 `token_version + 1` 立即使旧 access token 失效
- 仓储层新增最小能力：
  - `UserRepository.List`
  - `UserRepository.Update`

### 7.4 ACL 管理接口（Draft）

- 新增管理员 ACL 管理接口：
  - `GET /api/v1/acl/rules`
  - `POST /api/v1/acl/rules`
  - `PUT /api/v1/acl/rules/:id`
  - `DELETE /api/v1/acl/rules/:id`
- 当前权限模型：
  - 全部接口仅 `admin`
- 当前实现边界：
  - 本轮只实现 ACL 规则管理 CRUD
  - 暂未接入文件访问运行时权限判定
- 当前规则模型支持：
  - `subject_type = user`
  - `effect = allow / deny`
  - `permissions = read / write / delete / share`
  - `priority`
  - `inherit_to_children`
- 当前查询语义：
  - `source_id` 必填
  - `path` 可选，当前按精确路径过滤
- 新增持久化表：
  - `acl_rules`

### 7.5 ACL 运行时生效（进行中）

- 新增 `ACLAuthorizer`，开始把 ACL 从“可配置”推进到“真实生效”
- 当前运行时判定语义：
  - `super_admin` 保留 runtime ACL bypass
  - `admin / operator / user` 进入 ACL 判定
  - `multi_user_enabled=false` 且该 source 没有 ACL 规则时普通用户继续放行
  - 一旦 source 存在显式 ACL 规则，普通用户即进入 ACL 判定，避免配置了读权限但写操作仍被放行
  - `multi_user_enabled=true` 时普通用户进入 ACL 判定
  - 当前默认策略为：未命中规则即拒绝
  - 当前匹配顺序为：`priority desc, id asc`
  - 当前路径匹配支持：
    - 精确路径
    - `inherit_to_children=true` 的父路径继承
- 当前已接入运行时 ACL 的能力：
  - `GET /api/v1/files`
  - `GET /api/v1/files/search`
  - `POST /api/v1/files/mkdir`
  - `POST /api/v1/files/rename`
  - `POST /api/v1/files/move`
  - `POST /api/v1/files/copy`
  - `DELETE /api/v1/files`
  - `POST /api/v1/files/access-url`
  - `GET /api/v1/files/download`
  - `POST /api/v1/upload/init`
  - `WebDAV` 基础读写访问
- 当前动作映射：
  - list / search / access-url / download / WebDAV GET/HEAD/OPTIONS/PROPFIND → `read`
  - mkdir / rename / move / upload init / WebDAV PUT/MKCOL/MOVE → `write`
  - copy / WebDAV COPY → `source: read` + `target: write`
  - delete / WebDAV DELETE → `delete`
- 当前列表 / 搜索语义：
  - 返回结果按 item 做 ACL 过滤
  - 被拒绝路径不会出现在结果中
- 当前验证：
  - `go test ./internal/interfaces/http -run 'Test(LocalFileACLReadFlow|LocalFileACLWriteAndUploadFlow|WebDAVACLForNormalUser)' -v`
  - `go test ./...`

### 7.6 ACL 运行时覆盖继续推进

- 回收站接口已接入运行时 ACL：
  - `GET /api/v1/trash`
  - `POST /api/v1/trash/:id/restore`
  - `DELETE /api/v1/trash/:id`
  - `DELETE /api/v1/trash?source_id=...`
- 当前回收站 ACL 语义：
  - `list`：按 `write or delete` 过滤可见项
  - `restore`：要求目标原路径具备 `write`
  - `delete one`：要求目标原路径具备 `delete`
  - `clear source`：只清理当前用户有 `delete` 权限的条目
- 上传会话权限边界已收紧：
  - `POST /api/v1/upload/finish`
  - `DELETE /api/v1/upload/sessions/:upload_id`
  - 非 `admin` 只能操作自己的上传会话
  - 越权返回 `PERMISSION_DENIED`
- 补充了 S3 显式 ACL 集成测试，覆盖：
  - `files list/search`
  - `access-url`
  - `download`
  - `upload init`
- 当前新增验证：
  - `go test ./internal/interfaces/http -run 'Test(LocalTrashACLManagementFlow|UploadFinishCancelPermissionBoundary|S3FileACLReadWriteFlow)' -v`
  - `go test ./...`

### 7.7 ACL 运行时继续扩展到 upload chunk / tasks / source navigation

- 上传分片接口已补 owner 边界：
  - `PUT /api/v1/upload/chunk`
  - 非 `admin` 只能为自己的 upload session 上传分片
  - 越权返回 `PERMISSION_DENIED`
- 离线任务接口已按 `save_path` 接入 ACL：
  - `POST /api/v1/tasks` → `write`
  - `GET /api/v1/tasks` / `GET /api/v1/tasks/:id` → `read`
  - `POST /api/v1/tasks/:id/pause` / `POST /api/v1/tasks/:id/resume` → `write`
  - `DELETE /api/v1/tasks/:id` → `delete`
- `GET /api/v1/sources?view=navigation` 已对普通用户按 ACL 收敛可见性：
  - 当前策略为：当用户在某个 source 上存在任意 `allow` 规则时，该 source 出现在导航列表中
  - `admin` 与单用户模式继续保持原有可见性
- 当前新增验证：
  - `go test ./internal/interfaces/http -run 'Test(UploadChunkOwnerBoundary|TaskSavePathACLFlow|NavigationSourcesACLVisibility)' -v`
  - `go test ./...`

### 7.8 离线任务 owner 模型落地

- `download_tasks` 已补真实 owner 持久化字段：
  - `user_id`
- `TaskService.Create` 现在会从 request auth 写入 `task.user_id`
- 离线任务权限模型已从“仅依赖 save_path ACL”收敛为：
  - `create`：继续要求 `save_path` 具备 `write`
  - `list`：`admin` 可见全部；普通用户仅可见自己的任务
  - `get / pause / resume / cancel`：`admin` 可操作全部；普通用户仅可操作自己的任务
- 当前兼容语义：
  - 历史任务若 `user_id=0`，继续仅由 `admin` 可见/可操作
- 当前新增验证：
  - `go test ./internal/application/service -run TestTaskServiceCreatePersistsOwnerID -v`
  - `go test ./internal/interfaces/http -run 'Test(TaskOwnerIsolationFlow|TaskSavePathACLFlow)' -v`
  - `go test ./...`

### 7.9 分享链接文件 MVP

- 新增分享链接持久化模型：
  - `share_links`
- 新增分享管理接口：
  - `GET /api/v1/shares`
  - `POST /api/v1/shares`
  - `DELETE /api/v1/shares/:id`
- 新增公开访问入口：
  - `GET /s/:token`
- 当前分享语义：
  - 仅支持**文件分享**
  - 创建分享要求目标路径具备 `share` ACL 权限
  - 分享列表仅返回当前用户自己创建的分享
  - 非 owner 不能删除别人的分享
  - 支持可选过期时间
  - 支持可选访问密码
  - 公开访问成功后统一 `302` 跳转到后端受控下载地址
  - 公开访问已支持 local / S3 文件下载链路复用
- 当前错误语义：
  - 无密码访问受保护分享：`SHARE_PASSWORD_REQUIRED`
  - 密码错误：`SHARE_PASSWORD_INVALID`
  - 已过期：`SHARE_EXPIRED`
- 当前明确未纳入：
  - 目录分享
  - 分享浏览页 / 公开目录列表
  - 分享编辑 / 二次更新
- 当前新增验证：
  - `go test ./internal/interfaces/http -run 'TestShare(FileLifecycle|OwnerBoundaryAndACL|PasswordProtectedAccess|ExpiredAccess)' -v`
  - `go test ./...`

### 7.10 分享目录浏览 Draft

- 分享能力已从“仅文件下载”扩展为“文件下载 + 目录公开浏览”
- `POST /api/v1/shares` 现在允许目录路径创建分享
- `ShareLink.is_dir` 现在按真实目标类型持久化
- `GET /s/:token` 当前统一语义：
  - 文件分享：保持 `302` 跳转到后端受控下载地址
  - 目录分享根：返回 `200 + JSON` 目录列表
  - 目录分享子目录：支持 `?path=/subdir` 返回子目录列表
  - 目录分享内文件：支持 `?path=/subdir/file.ext` 返回 `302` 下载
- 新增目录分享边界约束：
  - `path` 必须以 `/` 开头
  - `path` 为相对于分享根的路径
  - 包含 `..` 或越界访问时返回 `PATH_INVALID`
- 当前实现已同时覆盖：
  - local driver
  - s3 driver
- 当前新增验证：
  - `go test ./internal/interfaces/http -run 'TestShare(DirectoryBrowseAndDownload|DirectoryPathBoundary)' -v`
  - `go test ./internal/interfaces/http -run 'TestS3Share(DirectoryBrowseAndRedirect|DirectoryPathBoundary)' -v`
  - `go test ./internal/interfaces/http -run 'TestShare(FileLifecycle|OwnerBoundaryAndACL|PasswordProtectedAccess|ExpiredAccess|DirectoryBrowseAndDownload|DirectoryPathBoundary)' -v`
  - `go test ./...`

### 7.11 分享管理增强（详情 / 编辑）

- 新增分享管理接口：
  - `GET /api/v1/shares/:id`
  - `PUT /api/v1/shares/:id`
- 当前详情语义：
  - 仅 owner 可查看自己的分享详情
  - 非 owner 访问返回 `PERMISSION_DENIED`
- 当前编辑语义：
  - 支持更新访问密码
  - 支持清空访问密码
  - 支持重设过期时间
  - 支持清空过期时间
  - 暂不支持修改 `source_id` / `path` / `name`
- 当前撤销语义：
  - 继续复用 `DELETE /api/v1/shares/:id` 作为“提前失效 / 撤销分享”
- 当前新增验证：
  - `go test ./internal/interfaces/http -run 'TestShare(GetAndUpdateLifecycle|GetAndUpdateOwnerBoundary)' -v`
  - `go test ./internal/interfaces/http -run 'TestShare(FileLifecycle|GetAndUpdateLifecycle|OwnerBoundaryAndACL|GetAndUpdateOwnerBoundary|PasswordProtectedAccess|ExpiredAccess|DirectoryBrowseAndDownload|DirectoryPathBoundary)|TestS3Share(DirectoryBrowseAndRedirect|DirectoryPathBoundary)' -v`
  - `go test ./...`

### 7.12 公开目录分享返回增强

- `GET /s/:token` 在目录分享场景下新增前端直出字段：
  - `current_dir`
  - `breadcrumbs`
  - `pagination`
  - `preview_type`
- 目录分享当前新增查询参数：
  - `page`
  - `page_size`
  - `sort_by`
  - `sort_order`
- 当前语义：
  - `items` 返回当前页条目
  - `breadcrumbs` 已按“分享根 -> 当前目录”展开
  - `current_dir` 可直接用于目录页标题区
  - `pagination` 可直接用于页码器
  - `preview_type` 用于前端快速判断目录 / 图片 / 视频 / 文本等展示策略
- 当前实现已同时覆盖：
  - local driver
  - s3 driver
- 当前新增验证：
  - `go test ./internal/interfaces/http -run 'TestShareDirectoryBrowseAndDownload|TestS3ShareDirectoryBrowseAndRedirect' -v`
  - `go test ./...`

### 8. S3 Storage Driver MVP

本阶段目标是让 Yunxia 拥有第二个真实存储驱动，并优先保证前端不会被存储驱动切换阻塞。

#### 8.1 S3 source 能力

- 支持 `driver_type=s3`
- 支持配置字段：
  - `endpoint`
  - `region`
  - `bucket`
  - `base_prefix`
  - `force_path_style`
- 支持 secret patch 字段：
  - `access_key`
  - `secret_key`
- 支持 source test / create / update / detail
- source detail 中公开配置与敏感字段掩码分离返回

#### 8.2 S3 文件能力

- 在应用层引入最小 storage driver 抽象
- S3 driver 已接入：
  - 文件列表
  - 按名称搜索
  - 预签名下载地址生成
- `POST /api/v1/files/access-url` 对 S3 返回 presigned URL

#### 8.3 S3 上传能力

- 为上传链路增加 upload driver 抽象
- S3 上传已支持 multipart 直传初始化
- `upload/init` 返回：
  - `transport.mode = direct_parts`
  - `driver_type = s3`
  - `part_instructions`
- `upload/finish` 已支持消费前端回传的 part etag 并完成 multipart upload
- 上传会话已增加 `storage_data` 持久化字段，用于保存 multipart 状态

#### 8.4 服务装配与测试

- `backend/cmd/server/main.go` 已接入真实 S3 driver
- `backend/internal/interfaces/http/router_test.go` 已接入 fake S3 driver
- 新增并跑通 S3 集成测试：
  - `TestS3SourceCreateDetailAndFileAccessLifecycle`
  - `TestS3UploadInitAndFinishLifecycle`

#### 8.5 S3 文件操作增强

- `GET /api/v1/files/download` 对 S3 已支持后端鉴权后 `302` 跳转到 presigned URL
- `POST /api/v1/files/access-url` 对 S3 已统一返回后端 `/api/v1/files/download?...access_token=...`
- 新增 S3 显式搜索集成测试
- 新增 S3 永久删除能力
- 新增 S3 trash 语义：`delete_mode=trash` 时移动到 `/.trash/<timestamp>/...`
- 新增非 local driver 的隐藏目录过滤：`.trash` / `.system` 不再出现在列表与搜索结果中
- 新增 S3 rename / move / copy 能力
- 新增 S3 mkdir 能力，支持创建空目录标记对象
- 补充 S3 目录级 rename / move / copy 显式集成测试

新增并跑通的 S3 集成测试：

- `TestS3FileSearchLifecycle`
- `TestS3DownloadRedirectLifecycle`
- `TestS3AccessURLRedirectLifecycle`
- `TestS3PermanentDeleteLifecycle`
- `TestS3TrashLifecycle`
- `TestS3RenameMoveCopyLifecycle`
- `TestS3DirectoryRenameMoveCopyLifecycle`
- `TestS3MkdirLifecycle`

### 9. 抽象与数据结构调整

新增/调整的关键抽象：

- `backend/internal/domain/storage/driver.go`
  - 存储驱动探测接口
  - 文件驱动接口
  - 上传驱动接口
- `backend/internal/application/service/storage_driver.go`
  - 应用层对驱动依赖的装配选项
- `backend/internal/infrastructure/storage/s3_config.go`
  - S3 配置解析、公开配置提取、secret patch 处理
- `backend/internal/infrastructure/storage/s3_client_factory.go`
  - S3 SDK client 创建
- `backend/internal/infrastructure/storage/s3_driver.go`
  - S3 探测、列表、搜索、presign 下载、multipart 上传

数据库 / 持久化变更：

- `StorageSource.ConfigJSON` 现已承载 local 与 S3 两类配置
- `UploadSession` 新增 `StorageDataJSON`
- `UploadSessionModel` 新增 `StorageDataJSON`

### 10. 依赖变更

新增 S3 相关依赖：

- `github.com/aws/aws-sdk-go-v2`
- `github.com/aws/aws-sdk-go-v2/config`
- `github.com/aws/aws-sdk-go-v2/credentials`
- `github.com/aws/aws-sdk-go-v2/service/s3`
- `github.com/aws/smithy-go`

对应文件：

- `backend/go.mod`
- `backend/go.sum`

### 11. 已完成验证

本阶段已完成的关键验证包括：

- `go test ./internal/infrastructure/downloader`
- `go test ./internal/interfaces/http -run TestTaskLifecycle -v`
- `go test ./internal/interfaces/http -run TestS3SourceCreateDetailAndFileAccessLifecycle -v`
- `go test ./internal/interfaces/http -run TestS3UploadInitAndFinishLifecycle -v`
- `go test ./...`
- `go test ./internal/application/service ./internal/interfaces/http -run 'Test(SystemServiceGetStatsAggregatesLocalSourcesAndTasks|SystemStatsRequireAdminAndReturnAggregates)' -v`
- `go test ./internal/interfaces/http -run 'Test(LocalTrashLifecycle|S3TrashClearLifecycle)' -v`
- `go test ./internal/application/service ./internal/interfaces/http -run 'Test(UserServiceManagementLifecycle|UserManagementRequireAdminAndLifecycle)' -v`
- `go test ./internal/application/service ./internal/interfaces/http -run 'Test(ACLServiceManagementLifecycle|ACLManagementRequireAdminAndLifecycle)' -v`

当前基线结论：

- local 主链路可用
- WebDAV（local）可用
- 离线下载 + pause/resume 可用
- S3 source / list / search / mkdir / rename / move / copy / delete（trash + permanent）/ access-url / download redirect / upload init / finish 可用
- 回收站 list / restore / delete one / clear source 可用
- 用户管理 list / create / update / reset-password / revoke-tokens 可用
- ACL 规则 list / create / update / delete 可用

### 12. 当前未完成项 / 后续建议

当前仍未纳入本轮完成范围的内容：

- S3 WebDAV
- OneDrive driver
- 分享公开页 UI / 短链管理增强
- 审计 / 搜索等高阶能力

建议后续继续顺序：

1. 继续补前端可能依赖的分享公开页增强（例如访问统计 / 最近访问时间 / 预览页元信息）
2. 若继续扩存储能力，再评估 S3 WebDAV / OneDrive driver
3. 再推进审计 / 搜索等高阶能力

### 13. 后端 Docker / Compose 部署配置

- 新增容器化相关文件：
  - `backend/Dockerfile`
  - `backend/.dockerignore`
  - `backend/.env.example`
  - `backend/docker/aria2.Dockerfile`
  - `backend/docker/aria2.entrypoint.sh`
  - `docker-compose.backend.yml`
- 后端镜像方案：
  - Go 多阶段构建
  - 运行时基础镜像为 `debian:bookworm-slim`
  - 容器默认数据目录为 `/app/data`
  - 默认健康检查为 `GET /api/v1/health`
- Aria2 侧车方案：
  - 使用自建 `alpine + aria2` 镜像
  - 默认配置目录 `/config`
  - 默认下载目录 `/downloads`
  - 支持通过环境变量覆盖 RPC secret、监听端口、并发下载数
- Compose 编排能力：
  - 新增 `backend` + `aria2` 双服务编排
  - 新增命名卷：
    - `backend-data`
    - `backend-downloads`
    - `aria2-config`
  - `backend` 与 `aria2` 共享 `/downloads`，便于后续把 Yunxia 自定义 local source 指向该目录
- 环境变量模板：
  - `backend/.env.example` 已补充 compose 启动示例
  - 已补充常用宿主机端口与 Aria2 参数模板
- 补充运行说明文档：
  - `docs/backend-docker-quickstart.md`
- 当前已完成验证：
  - `docker compose -f docker-compose.backend.yml config`
  - `docker build -f backend/Dockerfile backend`
  - `docker build -f backend/docker/aria2.Dockerfile backend`
  - `docker compose -f docker-compose.backend.yml up -d --build`
  - `docker compose -f docker-compose.backend.yml ps`
  - `Invoke-WebRequest -UseBasicParsing http://127.0.0.1:8080/api/v1/health`
  - `Invoke-RestMethod -Method Post http://127.0.0.1:6800/jsonrpc`（`aria2.getVersion`）
  - `cd backend && go test ./...`
- 当前已知限制：
  - 该限制已在 `14.9` 中解除：离线下载现在先进入本地 staging，再由后端导入目标挂载源

### 14. 统一存储 / VFS 第一阶段落地

#### 14.1 source 双路径模型

- `storage_sources` 已补 `mount_path`
- 当前 source 语义拆分为：
  - `mount_path`：挂载到统一虚拟目录树的位置
  - `root_path`：源内起始目录
- 默认本地源稳定挂载为 `/local`
- `mount_path` 当前要求：
  - 绝对路径
  - 规范化
  - 全局唯一

#### 14.2 VFS 核心与 v2 文件接口

- 新增统一虚拟目录树核心：
  - 最长前缀路径解析
  - 挂载注册表
  - 纯虚拟目录投影
  - 名称冲突检查
- 新增 v2 文件接口：
  - `GET /api/v2/fs/list`
  - `GET /api/v2/fs/search`
  - `GET /api/v2/fs/download`
  - `POST /api/v2/fs/access-url`
  - `POST /api/v2/fs/mkdir`
  - `POST /api/v2/fs/rename`
  - `POST /api/v2/fs/move`
  - `POST /api/v2/fs/copy`
  - `DELETE /api/v2/fs`
- 当前关键语义：
  - 北向统一使用 `virtual_path`
  - 纯虚拟目录可读不可写
  - 同父目录下文件 / 目录 / 挂载点 / 虚拟节点统一占名
  - S3 下载在 v2 下继续走 `302 -> presigned URL`

#### 14.3 上传迁移到 virtual path

- `POST /api/v1/upload/init` 现已兼容：
  - 旧模式：`source_id + path`
  - 新模式：`target_virtual_parent_path`
- 上传会话新增快照字段：
  - `target_virtual_parent_path`
  - `resolved_source_id`
  - `resolved_inner_parent_path`
- 分片上传与 finish 阶段继续复用现有 local / s3 传输协议

#### 14.4 业务模块虚拟路径快照

- ACL 规则新增：
  - `virtual_path`
- Share 新增：
  - `target_virtual_path`
  - `resolved_source_id`
  - `resolved_inner_path`
- Task 新增：
  - `save_virtual_path`
  - `resolved_source_id`
  - `resolved_inner_save_path`
- Trash 新增：
  - `original_virtual_path`
  - restore 返回 `restored_virtual_path`
- 当前 ACL runtime 已优先按 `virtual_path` 判定，旧 `source_id + path` 作为迁移兼容

#### 14.5 本轮新增验证

- `go test ./internal/interfaces/http -run 'TestVFSUpload' -v`
- `go test ./internal/interfaces/http -run 'Upload' -v`
- `go test ./internal/application/service ./internal/interfaces/http -run 'Test(ACL|Share|Task|Trash)' -v`
- `go test ./...`

#### 14.6 Docker 构建兼容性补强

- `backend/Dockerfile` 新增可配置基础镜像参数：
  - `GO_IMAGE`
  - `RUNTIME_IMAGE`
- `backend/docker/aria2.Dockerfile` 新增可配置基础镜像参数：
  - `ARIA2_BASE_IMAGE`
- `docker-compose.backend.yml` 现在会透传：
  - 基础镜像选择参数
  - `HTTP_PROXY / HTTPS_PROXY / NO_PROXY`
  - `http_proxy / https_proxy / no_proxy`
- 目的：
  - 兼容测试机已缓存基础镜像
  - 兼容受限网络下的 docker build 代理透传
  - 避免因固定镜像 tag 或代理不可达导致部署验证被阻塞

#### 14.7 Docker Compose 构建代理修正

- 问题现象：
  - 测试机启用 `http_proxy=http://127.0.0.1:7890` 后，`docker compose build` 会把该代理地址透传进 build 容器。
  - build 容器内的 `127.0.0.1` 指向容器自身，不是宿主机，导致 `apt-get update` / `apk add` 无法连接代理并中断构建。
- 修正：
  - `docker-compose.backend.yml` 为 `backend` 与 `aria2` 的 build 阶段新增可配置网络：
    - 默认 `YUNXIA_DOCKER_BUILD_NETWORK=host`
    - 允许在不支持 host build network 的环境覆盖为 `default`
  - `backend/.env.example` 补充该变量说明。
- 目的：
  - 让 Linux 测试机上通过本地代理解除网络限制后，仓库原生 Compose 构建流程可以继续完成。

#### 14.8 ACL 显式规则生效修正

- 问题现象：
  - 在默认 `multi_user_enabled=false` 状态下，即使管理员已经为普通用户创建了显式 ACL 规则，运行时仍会整体 bypass ACL。
  - 表现为普通用户仅被授予目录读取权限后，仍可在该目录下执行 `mkdir` 等写操作。
- 修正：
  - ACL 判定器现在会先加载 source 维度规则。
  - 仅当 `multi_user_enabled=false` 且该 source 没有任何 ACL 规则时才保留单用户兼容放行。
  - 一旦存在显式 ACL 规则，普通用户会按规则进入默认拒绝判定；`super_admin` 仍保留 runtime ACL bypass。
- 新增回归测试：
  - `TestVFSMkdirDeniedWhenUserOnlyHasReadACL`

#### 14.9 离线下载 staging 与目标存储源导入

- 离线下载执行策略调整为：
  - Aria2 只下载到后端本地 staging 目录
  - 后端检测任务完成后，将 staging 文件导入目标挂载源
  - 导入成功后清理该任务 staging 目录
- local 目标导入：
  - 同盘优先 `rename`
  - 跨盘或 rename 失败时 fallback 为 copy + remove
  - 目标文件已存在时拒绝覆盖
- S3 目标导入：
  - 新增 `S3Driver.ImportFile`
  - 后端从 staging 读取文件并 `PutObject` 到 S3
  - 目标对象已存在时拒绝覆盖
- Task 数据结构新增：
  - `target_virtual_parent_path`
  - `staging_dir`（仅内部持久化，不返回前端）
- `POST /api/v1/tasks` 现在支持两种目标指定方式：
  - 推荐：`target_virtual_parent_path`
  - 兼容：`source_id + save_path`
- 后端启动时会注册后台同步 worker：
  - 周期性刷新下载器状态
  - 对完成任务执行导入
- 新增回归验证：
  - `TestTaskCreateDownloadsIntoStagingAndImportsCompletedLocalFile`
  - `TestTaskCreateSupportsNonLocalTargetByImportDriver`
  - `TestTaskCreateAcceptsTargetVirtualParentPath`
  - `go test ./...`

#### 14.10 离线下载共享 staging 与 VFS ACL 列表过滤修正

- 修正离线下载在 Docker Compose 部署下没有真正进入目标存储源的问题：
  - 新增 `YUNXIA_ARIA2_DOWNLOAD_DIR`
  - backend 与 aria2 共同使用 `backend-downloads:/downloads`
  - Task staging 根目录改为 `${YUNXIA_ARIA2_DOWNLOAD_DIR}/staging`
  - 避免 Aria2 将文件下载到只有 aria2 容器可见的位置，导致后端无法导入
- 修正 VFS 列表越权展示问题：
  - `VFSService.List` 现在会对真实挂载目录下的子项执行 ACL read 过滤
  - 未授权文件不再出现在 `/api/v2/fs/list`
  - 返回项的 `can_delete` 会按 delete 权限收敛
- 新增回归验证：
  - `TestTaskStagingRootUsesSharedAria2DownloadDir`
  - `TestLoadReadsAria2DownloadDir`
  - `TestVFSListFiltersUnauthorizedMountedChildren`
  - `go test ./...`
  - `docker compose -f docker-compose.backend.yml config`

#### 14.11 离线任务终态实时指标清理

- 修正已完成任务仍返回下载中实时指标的问题：
  - `completed` 任务持久化刷新时会清空 `speed_bytes` / `eta_seconds`
  - API 输出终态任务时统一返回 `speed_bytes=0`、`eta_seconds=null`
  - 对历史已完成但残留速度 / ETA 的任务同样在响应层做兼容清理
- 新增回归验证：
  - `TestTaskCompletedClearsRealtimeDownloadMetrics`
  - `TestTaskGetSanitizesPersistedCompletedRealtimeMetrics`

#### 14.12 存储源路径参数错误响应修正

- 修正创建 / 测试 / 更新 local 存储源时路径参数错误被映射为 500 的问题。
- 当请求缺少 `config.base_path`，或 `root_path` / `mount_path` 等路径字段非法时：
  - 返回 HTTP `400`
  - 错误码为 `PATH_INVALID`
- 明确 local 源语义：
  - `config.base_path` 是物理宿主路径
  - `root_path` 是源内逻辑根路径
- 新增回归验证：
  - `TestLocalSourceCreateInvalidPathReturnsClientError`

#### 14.13 本地存储源 base_path 与 WebDAV slug 修正

- 修正创建 local 存储源时 `config.base_path` 不存在仍返回 201 且自动创建目录的问题。
  - 用户创建 / 测试 / 更新 local 源时，`config.base_path` 必须已存在且是目录。
  - 不存在或不是目录时返回 HTTP `400`，错误码 `PATH_INVALID`。
  - 默认本地源仍由系统初始化流程创建，不受该限制影响。
- 修正多个中文命名 local 源会生成相同 `webdav_slug=source-local`，导致数据库唯一约束错误并返回 500 的问题。
  - 创建源时后端会对 `webdav_slug` 自动去重，例如 `source-local`、`source-local-2`。
- 新增 / 更新回归验证：
  - `TestLocalSourceCreateRejectsMissingBasePath`
  - `TestSourceCRUDAndNavigationLifecycle`

#### 14.14 VFS ACL 根挂载过滤、本地只读源与任务错误清理

- 修正多用户开启后 `/api/v2/fs/list?path=/` 会泄露未授权挂载点名称的问题。
  - VFS 根目录和纯虚拟目录投影现在先按 `CanSeeSource` 过滤可见挂载源。
  - 与 `/api/v1/sources?view=navigation` 的可见性保持一致。
- 修正本地源实际不可写时的能力与错误表现：
  - local / VFS 列表在父目录不可写时返回 `can_delete=false`。
  - mkdir 等写操作命中只读本地源时返回 HTTP `403 SOURCE_READ_ONLY`，不再暴露底层 `read-only file system` 500。
- 修正离线下载任务已完成但仍残留旧 `error_message` 的问题：
  - 导入成功后清空任务错误信息。
  - 历史 `completed` 任务响应层也会把 `error_message` 收敛为 `null`。
- 新增 / 更新回归验证：
  - `TestNavigationSourcesACLVisibility`
  - `TestVFSListLocalReadOnlyDirectoryDisablesDeleteCapability`
  - `TestFileMkdirReadOnlyLocalSourceReturnsSourceReadOnly`
  - `TestTaskCompletedClearsStaleDownloaderErrorMessage`
  - `TestTaskGetSanitizesPersistedCompletedErrorMessage`

---

## 维护约定

后续如继续推进后端开发，建议按以下粒度追加记录：

- 新增模块
- 重要接口能力变化
- 数据结构 / 表结构变更
- 依赖升级或新增
- 回归验证结果
