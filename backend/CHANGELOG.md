# Yunxia Backend Changelog

> 说明：当前后端仍处于快速迭代期，先按“里程碑 + 能力范围”记录整体变更，而不是按正式版本号切分。

## 当前快照

- 后端根目录：`backend/`
- 技术栈：Go 1.25、Gin、GORM、PostgreSQL、JWT、bcrypt、Aria2、qBittorrent、AWS SDK for Go v2
- 当前状态：已完成全局权限模型重构；本地存储主链路、S3 MVP、离线下载、RSS/qBittorrent、通知告警、分享/ACL/上传链路已全部接入新权限模型
- 当前验证：`cd backend && go test ./...` 已通过
- 前端联调：2026-05-01 测试负责人确认 RSS MVP、RSS 无人值守端到端联调与前序待回归项已完成，`backend/FRONTEND_HANDOFF.md` 前端适配状态关闭

---

## 2026-05-07

### 分享绑定 Metadata VFS Node 阶段

- `ShareLink` / `ShareView` 新增 `target_vfs_node_id` 快照字段，分享创建时会按 `target_virtual_path` 解析并绑定 metadata VFS node，后续路径改名/移动时可继续以 node identity 追踪分享目标。
- 分享创建前会对目标路径的父级目录执行按需 metadata 懒刷新，确保 local/S3/PikPak 等挂载源中已存在但尚未入库的目录/文件可被解析为 VFS node。
- 生产依赖注入与 HTTP workflow 测试路由均为 `ShareService` 注册 metadata VFS reader/sync service；未注入时保持兼容，`target_vfs_node_id` 为空。
- 文档同步：更新 `backend/API_CONTRACT.md` 的 `ShareView` 字段说明；本阶段新增字段为向后兼容展示字段，不要求前端立即适配。

### VFS 标签最小后端能力阶段

- 新增 `VFSTagService` 与 HTTP handler，开放用户自有标签 CRUD：
  - `GET /api/v1/tags`
  - `POST /api/v1/tags`
  - `PATCH /api/v1/tags/:id`
  - `DELETE /api/v1/tags/:id`
- 新增 VFS 节点标签绑定接口：
  - `GET /api/v2/fs/tags?path=...`
  - `POST /api/v2/fs/tags/attach`
  - `POST /api/v2/fs/tags/detach`
- 标签按当前登录用户隔离；绑定目标基于 metadata VFS node，目标 path 不存在时返回 `FILE_NOT_FOUND`，跨用户 tag 操作返回 `PERMISSION_DENIED`。
- 文档同步：更新 `backend/API_CONTRACT.md` 与固定前端交接文档 `backend/FRONTEND_HANDOFF.md`。
- 新增 service / HTTP workflow 测试覆盖标签 CRUD、节点绑定/解绑、跨 owner 拒绝、缺失绑定稳定错误。

### 元数据化 VFS API 读路径切换阶段

- `/api/v2/fs/list` 开始优先使用 metadata VFS 读模型：根目录 / 纯虚拟目录从 `vfs_nodes` 返回，挂载目录进入时按需懒刷新当前目录直接子项，再从 DB 输出。
- `/api/v2/fs/search` 改为基于 metadata VFS path prefix 搜索，不再直接调用底层 source search；搜索前会对目标目录做一次 best-effort 懒刷新。
- 新增内置 local metadata indexer，使本地源已有文件可以通过 metadata VFS 懒刷新入库；S3/PikPak 继续通过已注册 `FileDriver` bridge 进入 metadata sync。
- 懒刷新时保护控制面节点：`mount` / `virtual_dir` 不会被底层同名文件覆盖，也不会因为底层 list 未返回而被标记 missing，从而支持 root/local 源子目录继续挂载其他存储源。
- VFS metadata 列表继续执行 ACL 过滤与能力收敛：未授权挂载点及其纯虚拟父目录不展示，本地只读目录返回 `can_delete=false`，driver capability 会约束 `can_download/can_delete`。
- 更新 `backend/API_CONTRACT.md` 中 VFS list/search 的 metadata 读模型说明。

### 元数据化 VFS Source Mount 控制面阶段

- 新增 `MetadataVFSMountService`，在 source mount 同步时维护父级 `virtual_dir`、挂载点 `vfs_nodes(kind=mount)` 与 `vfs_mounts` 记录，`root_locator_json` 只保存 source/root/config-root 快照并过滤 secret。
- `SourceService` 在注入 mount syncer 后会于 create/update/delete 路径同步或禁用 metadata mount；create/update 的 source 写入与 mount 同步通过事务提交，避免 source 成功但 VFS 控制面缺失。
- 后端启动会对现有 source 执行一次 `SyncAllSourceMounts` bootstrap；单个坏 source 只记录 warning 并继续启动，后续新建/更新请求仍以同步失败为稳定错误返回。
- 默认 setup 本地 source 创建后也会同步 metadata mount；source disabled 或 mount_path 改名时保留原 mount node/子树，旧 `vfs_mount` 禁用并将旧 mount node 标记 `stale`。
- 新增 service 测试覆盖 mount 控制面创建、mount_path 改名保留旧节点并禁用旧 mount、disabled source 同步、bootstrap 单源失败不中断、source create/update/delete 调用 mount syncer，以及事务回滚。
- 本阶段仍不迁移 `/api/v2/fs` handler、不新增 HTTP route/DTO，不要求前端改动；`backend/API_CONTRACT.md` 已补充 source mutation 的 `METADATA_VFS_MOUNT_SYNC_FAILED` 错误码说明。

### 元数据化 VFS 业务模块最小接入阶段

- 新增 `MetadataVFSCommitService` 应用层提交端口，封装业务完成后维护 metadata VFS 控制面快照：懒创建父目录 path、upsert `storage_objects`、upsert file `vfs_nodes`。
- `UploadService` 在已注入 metadata committer 时，会在本地上传、server_chunk import、direct multipart 完成以及 fast-upload 命中后提交 file node/object；未注入时保持旧行为。
- `TaskService` 在 staging/import 成功后提交导入文件的 file node/object，并按实际导入路径补齐父目录节点；metadata 提交失败会让任务落入 failed，错误信息固定为安全哨兵，避免任务伪 completed 或泄露物理路径/secret。
- `MetadataVFSCommitService` 对显式 locator JSON 做 canonical marshal 与敏感字段脱敏，默认 locator 仅记录 driver 内部逻辑 path，不记录本机物理路径或 provider token。
- RSS 仍不直接依赖 metadata 底层，继续通过 TaskService 创建下载任务；本阶段不新增 route/DTO，不迁移 `/api/v2/fs` handler，不要求前端改动。
- 新增 service 测试覆盖本地上传、server_chunk import、direct multipart、fast-upload metadata 提交，任务导入 metadata 提交，metadata 提交失败时任务不落 completed，以及 locator JSON 稳定脱敏。

### 元数据化 VFS Lazy Index / Sync 阶段

- 新增 `MetadataVFSSyncService` 懒索引/同步服务，支持按 metadata path 或 node 刷新 source-backed/mount 目录的直接子项，服务层仅依赖 domain repository 与 domain/storage driver/indexer interface，不导入 GORM 或 HTTP。
- 新增 domain/storage `RemoteIndexer` 抽象与 `FileDriver.List` 兼容桥，driver/local list 结果可 upsert 为 VFS nodes；文件条目会同步 upsert `StorageObject`，并让 `VFSNode.ObjectID` 指向对象。
- 同步状态语义落地：成功刷新更新目标目录 `indexed`，远端未再次看到的 active child 标记 `missing`，provider/list 失败时保留旧 DB 视图并把目标目录标记 `error`，同名冲突标记 `conflict` 并返回稳定冲突错误。
- 补充 storage object locator upsert 能力：repository interface/GORM 实现新增 `FindByLocator`、`UpsertByLocator`，并为 locator 增加唯一索引，provider locator 继续以 JSON/string 透传。
- 新增 service 单元测试覆盖首次刷新创建节点、二次刷新更新 metadata/object、远端删除标 missing、driver 失败保留旧节点、同名冲突、文件 object 绑定；新增 repository 测试覆盖 storage object locator upsert。
- 本阶段仍未迁移 `/api/v2/fs` 对外行为，未新增前端 route/DTO，因此 `backend/API_CONTRACT.md` 与 `backend/FRONTEND_HANDOFF.md` 暂不变更。

### 元数据化 VFS Metadata Service 阶段

- 新增 `MetadataVFSService` 控制面服务骨架，服务层只依赖 VFS metadata repository interface，不直接依赖 GORM 或 HTTP：
  - root `EnsureRoot`
  - path `ResolveNode`
  - DB metadata children list / name-path prefix search
  - metadata `mkdir` / `rename` / `move` / soft delete
  - node tag attach / list helper
- 当前阶段的 rename / move 只更新同一 metadata tree 内的 path 快照；跨挂载 / 跨 source 真实数据面移动暂不触碰，留给后续 driver import / lazy index 阶段。
- 加固 metadata tree mutation 边界：rename/move/delete 拒绝 root 或挂载点等不应直接改动的节点，move/rename 会在更新前预检目标子树 path 冲突，pending/syncing/conflict/missing/error 文件节点不标记为可下载。
- 新增 service 单元测试覆盖 root/list、mkdir 同名冲突与软删除重建、search、rename 子树 path 更新与冲突、move 子树 path 更新与预检冲突、soft delete、tag attach/list。
- 本阶段未迁移 `/api/v2/fs` HTTP 行为，未新增前端可见 route/DTO，因此 `backend/API_CONTRACT.md` 暂不变更。

### 元数据化 VFS Schema / Repository 阶段

- 新增元数据化 VFS 控制面最小持久化基础：
  - domain entity：`VFSNode`、`StorageObject`、`VFSMount`、`VFSTag`、`VFSNodeTag`
  - repository interface/filter：VFS node/object/mount/tag 基础 CRUD、path 查询、parent 子节点列表、path/mount/tag upsert、node/tag 绑定
  - GORM models 与 AutoMigrate 列表：新增 VFS node、storage object、mount、tag、node_tag 持久化模型
- 持久化约束与约定：
  - storage object locator 与 mount root locator 使用 PostgreSQL `jsonb`
  - VFS node 保留 `parent_id + name` 与 `path` 快照双模型，并通过未删除节点的 partial unique index 保护 path / sibling 名称冲突
  - VFS node repository 的 `Delete` 语义为软删除节点及其 path 子树，避免硬删导致活跃子节点悬空，并允许同父同名节点在软删除后重新创建
  - repository 实现继续使用 `dbFor(ctx, db)`、`normalizeGormError`、`Select/Omit` 更新模式
- 本阶段仅新增后端 schema/repository 能力，未暴露新 HTTP API，因此 `backend/API_CONTRACT.md` 暂不变更。

---

## 2026-05-06

### 元数据化 VFS 与扁平数据面重构方案

- 新增元数据化 VFS 设计任务与 PRD：
  - `.trellis/tasks/05-06-metadata-vfs-flat-storage/prd.md`
- 新增固定架构文档：
  - `docs/research/metadata-vfs-flat-storage-architecture.md`
- 明确后续后端重构目标：
  - 控制面完全虚拟化：目录、挂载、ACL、分享、标签、上传/离线下载/RSS 任务目标走 Yunxia VFS 元数据。
  - 数据面扁平化抽象：local / S3 / PikPak 等底层 source 只负责对象/文件内容读写、远端列举、直链、导入和 provider 原生能力。
  - 第三方网盘已有目录通过实时 list + 懒索引 + 后台同步进入 VFS，而不是全量扫描后才可见。
- 文档索引同步：
  - 更新 `DOCS-INDEX.md`
  - 更新 `docs/README.md`

---

### PostgreSQL-only 数据库迁移

- 后端数据库从 SQLite 干净切换为 PostgreSQL-only：
  - 移除 SQLite driver 与文件型 DSN 默认值。
  - 新增 PostgreSQL runtime，统一封装连接池、健康检查、AutoMigrate 和关闭。
  - `docker-compose.backend.yml` 新增 `postgres` service 与 `postgres-data` 命名卷，后端等待 PostgreSQL healthcheck 后启动。
- 数据库配置增强：
  - 新增 `YUNXIA_DATABASE_AUTO_MIGRATE`
  - 新增连接池配置：`MAX_OPEN_CONNS`、`MAX_IDLE_CONNS`、`CONN_MAX_LIFETIME`、`CONN_MAX_IDLE_TIME`
  - 新增 `YUNXIA_DATABASE_SLOW_THRESHOLD`
- 持久化层抽象增强：
  - 新增 domain-level `Transactor` 事务端口和 GORM 实现。
  - 仓储层通过 `dbFor(ctx, db)` 复用事务上下文，上层无需感知 GORM/PostgreSQL。
  - PostgreSQL 唯一约束、外键/检查约束等错误统一映射为稳定 repository sentinel。
- 模型优化：
  - JSON-like 字段的 PostgreSQL 列类型切换为 `jsonb`。
  - 部分“未解析/无关联”的 source 引用改为 nullable model 字段，domain 转换层继续保持兼容。
  - bool/zero-value 写入继续通过显式字段选择和回归测试保护。
- 测试策略调整：
  - 仓储/HTTP 集成测试切换为 `YUNXIA_TEST_DATABASE_DSN` + 独立 PostgreSQL schema。
  - 未配置测试 DSN 时跳过真实 PostgreSQL 集成测试，不再回退 SQLite。
- 文档同步：
  - 更新 `docs/research/postgresql-only-database-migration.md`
  - 更新 `docs/runbooks/backend-docker-quickstart.md`
  - 更新 `.trellis/spec/backend/database-guidelines.md`

---

## 2026-04-30

### RSS 无人值守可用性增强

- RSS 源自动刷新增强：
  - 新增 `health_status`、`consecutive_failures`、`last_success_at`、`next_refresh_at`、`last_refresh_status`、`last_refresh_stats`
  - 连续失败自动退避，达到阈值后进入 `circuit_open`，成功刷新后恢复 `ok`
  - 新增 `POST /api/v1/rss/sources/refresh-all`，单源失败不影响其他源
- RSS 订阅可解释性增强：
  - 新增 `POST /api/v1/rss/subscriptions/:id/preview`
  - 返回每个已有 item 的 `matched` / `missing` / `excluded` 解释
- RSS item 重试与无人值守自愈：
  - 新增状态 `retry_pending`、`completed`、`needs_attention`
  - 新增 `retry_count`、`max_retry_count`、`last_attempt_at`、`next_retry_at`、`retry_reason`
  - 新增 `POST /api/v1/rss/items/:id/reprocess` 与 `POST /api/v1/rss/items/:id/retry`
  - 自动 retry worker 按 5m / 30m / 2h 退避，默认最多 3 次
  - task `completed` / `failed` / `canceled` 会回写 RSS item
  - 已有关联非终态 task 的 item 不会重复入队
  - 自动刷新不会绕过 `retry_pending` / `needs_attention` / `completed` 状态重复入队；task `canceled` 会进入 `needs_attention`
  - `refresh-all` 强制刷新所有启用源，只有并发刷新锁占用时才标记 `skipped`
- 文档同步：
  - 更新 `backend/API_CONTRACT.md`
  - 更新 `backend/FRONTEND_HANDOFF.md`
  - 更新 `.trellis/spec/backend/rss-guidelines.md`
- 新增 / 更新回归验证：
  - `TestRSSSourceFailureBackoffAndRecovery`
  - `TestRSSRefreshAllContinuesAfterSingleSourceFailure`
  - `TestRSSSubscriptionPreviewExplainsMatchedMissingExcluded`
  - `TestRSSManualRetryAndReprocess`
  - `TestRSSRetryWorkerBackoffAndMaxAttention`
  - `TestRSSTaskBacklinkUpdatesItemTerminalStates`
  - `TestRSSRetrySkipsActiveTaskToAvoidDuplicate`
  - `TestRSSRefreshAllForcesEnabledSourcesEvenWhenNotDue`
  - `TestRSSRefreshDoesNotRequeueCompletedOrRetryPendingItems`
  - `TestRSSTaskCanceledMovesItemToNeedsAttention`
  - `go test -count=1 ./...`

### RSS / qBittorrent 回归修正

- 修正 Mikan RSS 发布时间解析：
  - 兼容 Mikan `torrent/pubDate` 扩展字段
  - 同步识别 Mikan `torrent/link`，作为 `.torrent` 下载候选
  - 支持无时区 ISO 时间，例如 `2026-04-25T18:39:13.708`
  - 无时区时间按 `Asia/Shanghai` 解析后输出 RFC3339
- 修正 RSS 关键词短数字误命中：
  - `must_contain: ["05", "1080p"]` 中的 `05` 现在按集数语义只匹配标题集数
  - 不再因为 URL、torrent hash、发布时间等元信息包含 `05` 而误匹配第 02 集
  - 普通文本关键词仍可匹配标题和链接元数据
- 修正 `.torrent` URL 入队到 qBittorrent 后很快变成 `canceled` 的问题：
  - `.torrent` URL 现在由后端先下载 torrent 文件
  - 再通过 qBittorrent Web API multipart `torrents` 文件字段提交
  - qBittorrent tag 暂时不可见时不再立即映射为 `canceled`
- 补强任务终态错误信息：
  - 下载器返回 `failed` / `canceled` 且无错误原因时，后端补默认 `error_message`
  - 用户主动取消任务时持久化 `download canceled by user`
- 新增 / 更新回归验证：
  - `TestFetcherParsesMikanTorrentPubDate`
  - `TestRSSSubscriptionShortNumericKeywordMatchesEpisodeInTitleOnly`
  - `TestRSSSubscriptionShortNumericKeywordIgnoresDatesAndResolution`
  - `TestRSSSubscriptionShortNumericKeywordMatchesExplicitEpisodeForms`
  - `TestRSSSubscriptionShortNumericKeywordIgnoresTitleHash`
  - `TestRSSSubscriptionRegexModeCanStillMatchMetadata`
  - `TestQBittorrentClientAddTorrentURLUploadsDownloadedTorrentFile`
  - `TestQBittorrentClientAddTorrentURLFetchFailureDoesNotPostAdd`
  - `TestQBittorrentClientTellStatusKeepsMissingTagPending`
  - `TestTaskRefreshSetsTerminalErrorMessage`
  - `TestTaskCancelSetsTerminalErrorMessage`
  - `go test -count=1 ./...`

---

## 2026-04-29

### RSS 番剧订阅下载 MVP 与 qBittorrent 接入

- 新增 RSS 订阅下载后端主链路：
  - RSS 源：新增 / 列表 / 详情 / 更新 / 删除 / 手动刷新
  - RSS 订阅：新增 / 列表 / 详情 / 更新 / 删除 / 手动执行
  - RSS 条目：列表 / BT 条目手动入队 / 状态追踪
- 第一版 RSS 只自动处理 BT/magnet：
  - `magnet:?` → qBittorrent
  - `.torrent` URL → qBittorrent
  - 普通 HTTP/HTTPS RSS 条目标记为 `unsupported`，不自动创建 RSS 下载任务
- 保留 Aria2，新增 qBittorrent 下载器路由：
  - 普通 HTTP/HTTPS 离线下载继续走 Aria2
  - BT/magnet 离线下载走 qBittorrent
  - `DownloadTaskView` 新增 `downloader_type`
- RSS 目标目录统一基于 VFS：
  - 每个订阅固定 `target_virtual_parent_path`
  - 创建/更新订阅时校验 VFS backing storage、ACL 写权限、本地源只读状态
  - 下载完成后仍走 Yunxia staging/import 进入目标存储源
- 新增 qBittorrent Web API 客户端：
  - 登录 / 免登录内网模式
  - 添加 torrent/magnet
  - 查询状态
  - 暂停 / 恢复 / 删除
  - 健康检查
- Docker 后端编排新增 qBittorrent 侧车：
  - `backend/docker/qbittorrent.Dockerfile`
  - `backend/docker/qbittorrent.entrypoint.sh`
  - `docker-compose.backend.yml` 新增 `qbittorrent` 服务与共享下载卷
- 新增权限能力：
  - `rss.read`
  - `rss.manage`
- API 文档已补充 `/api/v1/rss/*` 与 `downloader_type`。
- 当前验证：
  - `cd backend && go test ./...`
  - `docker compose -f docker-compose.backend.yml config`
  - `bash -n backend/docker/aria2.entrypoint.sh backend/docker/qbittorrent.entrypoint.sh`

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

## 2026-05-02

### RSS 管理易用性、番剧模板与通知告警

- RSS 番剧识别与模板能力增强：
  - RSS item 新增并持久化 `parsed` 元数据：番剧名、季度、集数、字幕组、分辨率。
  - RSS subscription 新增 `directory_template` 与 `filename_template`。
  - `directory_template` 会把 RSS 下载目标渲染为 `target_virtual_parent_path` 下的安全相对子目录。
  - `filename_template` 会在 RSS 入队时渲染为离线任务 `target_filename` 快照。
  - 单文件任务完成后在后端 staging 导入点按 `target_filename` 重命名；模板未写扩展名时保留原文件扩展名，多文件任务仍保留原相对路径。
- RSS 管理易用性增强：
  - 新增 RSS 导入 / 导出：`GET /api/v1/rss/export`、`POST /api/v1/rss/import`。
  - 新增临时规则 preview：`POST /api/v1/rss/subscriptions/preview`。
  - 新增订阅复制、批量启停、条目批量忽略、条目批量重试接口。
  - 导入支持 `dry_run`、同 owner+URL source 复用、订阅目标 VFS 写权限重校验与逐项部分失败结果。
- 新增通知告警模块：
  - 新增 Webhook 通道管理：`/api/v1/notifications/channels`。
  - 新增通知事件列表与手动重试：`/api/v1/notifications/events`。
  - 新增通知 capability：`notification.read`、`notification.manage`。
  - RSS source 进入 `degraded` / `circuit_open`、RSS item 进入 `needs_attention`、RSS 下载完成时会持久化通知事件并尝试 Webhook 投递。
  - Webhook 投递失败会进入 `retry_pending` / `failed`，并保留 `attempts`、`next_attempt_at`、`last_error`。
- 文档同步：
  - 更新 `backend/API_CONTRACT.md`。
  - 更新 `backend/FRONTEND_HANDOFF.md`。
  - 更新 `docs/PROJECT-TODO.md`。
  - 新增 `.trellis/spec/backend/notification-guidelines.md` 并更新 spec index。
- 新增 / 更新回归验证：
  - `TestRSSExportConfigExcludesRuntimeFields`
  - `TestRSSImportConfigDryRunDoesNotPersist`
  - `TestRSSImportConfigReusesSameURLSourceAndCreatesSubscription`
  - `TestRSSImportConfigInvalidTargetFailsSubscriptionItem`
  - `TestRSSEnqueueFilenameTemplateTargetFilename`
  - `TestTaskTargetFilenameRenamesSingleStagedFile`
  - `TestTaskTargetFilenameIgnoredForMultiFileStaging`
  - `TestNotificationChannelConfigHidesSecret`
  - `TestNotificationDispatchFailureCanRetry`
  - `TestNotificationEventTypeFilterSkipsUnmatchedChannels`
  - `TestNotificationRepositoryPersistsChannelAndEvent`
  - `go test ./...`

---

## 维护约定

后续如继续推进后端开发，建议按以下粒度追加记录：

- 新增模块
- 重要接口能力变化
- 数据结构 / 表结构变更
- 依赖升级或新增
- 回归验证结果
