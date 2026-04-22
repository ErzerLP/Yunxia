# Unified Storage VFS Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 Yunxia 后端从 `source_id + path` 模型升级为基于 `virtual_path` 的统一虚拟目录树，并提供可测试的 VFS 核心、V2 文件接口和关键业务模型迁移基础。

**Architecture:** 保留现有 local / s3 driver，不直接重写底层存储能力；在其上新增 `MountRegistry + PathResolver + VirtualDirProjector + VFSService`。北向新增 V2 API 并统一只暴露 `virtual_path`，南向仍通过“最长前缀命中的 source + inner_path”执行真实读写。第一阶段只支持单挂载树，不实现 `.balance`、Alias union 与统一 WebDAV。

**Tech Stack:** Go 1.25、Gin、GORM、SQLite、现有 local/S3 driver、httptest

---

## 文件结构总览

### 新增文件

- `backend/internal/application/dto/vfs_dto.go`
- `backend/internal/application/service/vfs_types.go`
- `backend/internal/application/service/vfs_path.go`
- `backend/internal/application/service/vfs_registry.go`
- `backend/internal/application/service/vfs_service.go`
- `backend/internal/application/service/vfs_conflict.go`
- `backend/internal/interfaces/http/handler/vfs_handler.go`
- `backend/internal/interfaces/http/vfs_workflow_test.go`

### 重点修改文件

- `backend/internal/domain/entity/storage_source.go`
- `backend/internal/application/dto/source_dto.go`
- `backend/internal/application/service/source_service.go`
- `backend/internal/application/service/file_service.go`
- `backend/internal/application/service/local_storage_helpers.go`
- `backend/internal/application/service/service_test.go`
- `backend/internal/interfaces/http/router.go`
- `backend/internal/interfaces/http/router_test.go`
- `backend/internal/interfaces/http/handler/source_handler.go`
- `backend/internal/infrastructure/persistence/gorm/models.go`
- `backend/internal/infrastructure/persistence/gorm/source_repo_impl.go`
- `backend/internal/interfaces/http/storage_workflow_test.go`
- `backend/internal/application/dto/upload_dto.go`
- `backend/internal/application/dto/share_dto.go`
- `backend/internal/application/dto/task_dto.go`
- `backend/internal/application/dto/trash_dto.go`
- `backend/CHANGELOG.md`
- `backend/API_CONTRACT.md`

### 后续阶段预留修改文件

- `backend/internal/application/service/acl_service.go`
- `backend/internal/application/service/acl_authorizer.go`
- `backend/internal/application/service/share_service.go`
- `backend/internal/application/service/task_service.go`
- `backend/internal/application/service/upload_service.go`
- `backend/internal/application/service/trash_service.go`
- `backend/internal/interfaces/http/handler/upload_handler.go`
- `backend/internal/interfaces/http/handler/share_handler.go`
- `backend/internal/interfaces/http/handler/task_handler.go`
- `backend/internal/interfaces/http/handler/webdav_handler.go`

---

### Task 1: 给存储源补齐 `mount_path` 模型与迁移约束

**Files:**
- Modify: `backend/internal/domain/entity/storage_source.go`
- Modify: `backend/internal/application/dto/source_dto.go`
- Modify: `backend/internal/application/service/source_service.go`
- Modify: `backend/internal/infrastructure/persistence/gorm/models.go`
- Modify: `backend/internal/infrastructure/persistence/gorm/source_repo_impl.go`
- Modify: `backend/internal/interfaces/http/handler/source_handler.go`
- Modify: `backend/internal/interfaces/http/storage_workflow_test.go`

- [ ] 在 `StorageSource`、`StorageSourceModel`、`StorageSourceView`、`SourceUpsertRequest` 中新增 `MountPath` 字段，保持 `RootPath` 继续表示“源内起始路径”。
- [ ] 在 `source_service.go` 中为 create / update 引入 `mount_path` 规范化逻辑，要求绝对路径、清理 `.` / `..`、移除多余斜杠。
- [ ] 在 source 创建与更新流程中加入挂载唯一性校验；若 mount 已被其他 source 使用，则返回 `ErrSourceMountPathConflict`。
- [ ] 为本地默认 source 设置稳定的默认 `mount_path`（推荐 `/local`），避免多个 source 默认挂到 `/`。
- [ ] 修改 `storage_workflow_test.go`，补充 `mount_path` 的 create / detail / update 测试，确保 `root_path` 与 `mount_path` 语义不混淆。
- [ ] Run: `go test ./internal/interfaces/http -run TestStorageSourceLifecycle -v`
- [ ] Commit: `git commit -m "feat: add source mount path model"`

### Task 2: 实现 VFS 基础类型、路径规范化与最长前缀解析

**Files:**
- Create: `backend/internal/application/service/vfs_types.go`
- Create: `backend/internal/application/service/vfs_path.go`
- Modify: `backend/internal/application/service/service_test.go`

- [ ] 在 `vfs_types.go` 定义最小运行时结构：`MountEntry`、`ResolvedPath`、`VirtualEntryKind`。
- [ ] 在 `vfs_path.go` 实现统一路径工具：`normalizeVirtualPath`、`normalizeMountPath`、`splitParentName`、`isSubPath`。
- [ ] 在 `vfs_path.go` 实现最长前缀匹配函数：输入 `virtual_path` 和挂载列表，输出唯一命中的 `mount_path + inner_path`。
- [ ] 在 `service_test.go` 中新增失败测试：
  - `TestNormalizeMountPath`
  - `TestResolveVirtualPathByLongestPrefix`
  - `TestResolveVirtualPathFallsBackToPureVirtualParent`
- [ ] Run: `go test ./internal/application/service -run 'Test(NormalizeMountPath|ResolveVirtualPath)' -v`
- [ ] Commit: `git commit -m "feat: add vfs path resolver"`

### Task 3: 实现挂载注册表与虚拟目录投影

**Files:**
- Create: `backend/internal/application/service/vfs_registry.go`
- Modify: `backend/internal/application/service/service_test.go`
- Modify: `backend/internal/application/service/source_service.go`

- [ ] 在 `vfs_registry.go` 实现 `MountRegistry`，支持：
  - 从 `SourceRepository` 加载启用 source
  - 获取全部 mount
  - 查找某前缀下的直接子挂载
  - 检查 mount path 是否与现有源冲突
- [ ] 在 `vfs_registry.go` 实现“纯虚拟目录投影”：给定 `/docs` 返回 `team/`、`personal/` 这样的虚拟目录节点。
- [ ] 在 `service_test.go` 中新增失败测试：
  - `TestProjectVirtualChildrenForRoot`
  - `TestProjectVirtualChildrenForNestedPrefix`
  - `TestProjectVirtualChildrenDeduplicatesNames`
- [ ] 在 `source_service.go` 中为 create / update 的 mount path 校验复用 `MountRegistry` 或同一规则函数，避免双重语义。
- [ ] Run: `go test ./internal/application/service -run 'TestProjectVirtualChildren' -v`
- [ ] Commit: `git commit -m "feat: add vfs mount registry"`

### Task 4: 实现名称冲突检查与写路径落点判定

**Files:**
- Create: `backend/internal/application/service/vfs_conflict.go`
- Modify: `backend/internal/application/service/vfs_service.go`
- Modify: `backend/internal/application/service/file_service.go`
- Modify: `backend/internal/application/service/service_test.go`

- [ ] 在 `vfs_conflict.go` 实现“同父目录名称唯一”检查，冲突来源至少覆盖：
  - 当前真实目录已有同名文件或目录
  - 当前父目录下已有挂载点同名
  - 目标路径与更深层子挂载冲突
- [ ] 在 `vfs_service.go` 中实现 `ResolveWritableTarget`：
  - 若目标路径能唯一命中真实挂载，则返回可写 `ResolvedPath`
  - 若仅命中纯虚拟父目录，则返回 `ErrNoBackingStorage`
  - 若命中同父目录重名，则返回 `ErrNameConflict`
- [ ] 在 `service_test.go` 中新增失败测试：
  - `TestResolveWritableTargetAllowsMappedVirtualPath`
  - `TestResolveWritableTargetRejectsPureVirtualParent`
  - `TestResolveWritableTargetRejectsNameConflictWithMount`
- [ ] Run: `go test ./internal/application/service -run 'TestResolveWritableTarget|TestNameConflict' -v`
- [ ] Commit: `git commit -m "feat: add vfs conflict checks"`

### Task 5: 实现 VFS 目录列出与虚拟/真实结果合并

**Files:**
- Create: `backend/internal/application/dto/vfs_dto.go`
- Create: `backend/internal/application/service/vfs_service.go`
- Modify: `backend/internal/application/service/local_storage_helpers.go`
- Modify: `backend/internal/application/service/file_service.go`
- Modify: `backend/internal/application/service/service_test.go`

- [ ] 在 `vfs_dto.go` 定义 V2 文件项与响应：
  - `VFSItem`
  - `VFSListResponse`
  - `VFSSearchResponse`
  - `entry_kind`
  - `is_virtual`
  - `is_mount_point`
- [ ] 在 `vfs_service.go` 实现 `List(path)`：
  - 先投影虚拟挂载子目录
  - 再解析当前路径是否命中真实挂载
  - 若命中真实挂载，则读取真实目录内容
  - 最后合并并去重，冲突时挂载优先
- [ ] 复用 `buildFileItem` / `buildStorageEntryItem` 的逻辑为 VFS item 生成器；必要时新增 `buildVFSItemFromLocal` / `buildVFSItemFromStorageEntry`，避免污染 v1 DTO。
- [ ] 在 `service_test.go` 中新增失败测试：
  - `TestVFSListRootReturnsProjectedMounts`
  - `TestVFSListPureVirtualDirectoryReturnsOnlyProjectedChildren`
  - `TestVFSListRealAndVirtualChildrenMergedWithMountPriority`
- [ ] Run: `go test ./internal/application/service -run 'TestVFSList' -v`
- [ ] Commit: `git commit -m "feat: add vfs list service"`

### Task 6: 接入 V2 只读接口（list / search / download / access-url）

**Files:**
- Create: `backend/internal/interfaces/http/handler/vfs_handler.go`
- Modify: `backend/internal/interfaces/http/router.go`
- Modify: `backend/internal/interfaces/http/router_test.go`
- Create: `backend/internal/interfaces/http/vfs_workflow_test.go`
- Modify: `backend/cmd/server/main.go`

- [ ] 在 `vfs_handler.go` 暴露只读接口：
  - `GET /api/v2/fs/list`
  - `GET /api/v2/fs/search`
  - `GET /api/v2/fs/download`
  - `POST /api/v2/fs/access-url`
- [ ] 在 `router.go` 注册 `/api/v2/fs/*` 路由，并保持 v1 路由不受影响。
- [ ] 在 `main.go` 装配 `VFSService` 与 `VFSHandler`，注入 `SourceRepository`、ACLAuthorizer、driver registry、token service。
- [ ] 在 `vfs_workflow_test.go` 中新增集成测试：
  - `TestVFSListNestedMounts`
  - `TestVFSDownloadLocalByVirtualPath`
  - `TestVFSDownloadS3ByVirtualPathRedirect`
  - `TestVFSAccessURLByVirtualPath`
- [ ] Run: `go test ./internal/interfaces/http -run 'TestVFS' -v`
- [ ] Commit: `git commit -m "feat: add v2 fs read apis"`

### Task 7: 接入 V2 写接口（mkdir / rename / move / copy / delete）

**Files:**
- Modify: `backend/internal/application/service/vfs_service.go`
- Modify: `backend/internal/interfaces/http/handler/vfs_handler.go`
- Modify: `backend/internal/interfaces/http/vfs_workflow_test.go`
- Modify: `backend/internal/application/service/service_test.go`

- [ ] 在 `VFSService` 中实现：
  - `Mkdir`
  - `Rename`
  - `Move`
  - `Copy`
  - `Delete`
- [ ] 规则统一：
  - 所有入参使用 `virtual_path`
  - 通过 `ResolveWritableTarget` 落到真实 source + inner path
  - 同挂载内优先走底层原生语义
  - 跨挂载 `move` 退化为 `copy + delete`
- [ ] 在 `service_test.go` 中新增失败测试：
  - `TestVFSMkdirOnMappedPath`
  - `TestVFSMkdirRejectsPureVirtualParent`
  - `TestVFSRenameRejectsMountNameConflict`
  - `TestVFSMoveAcrossMountsFallsBackToCopyDelete`
- [ ] 在 `vfs_workflow_test.go` 中新增 HTTP 流程测试，验证所有写接口返回正确错误码：
  - `NAME_CONFLICT`
  - `NO_BACKING_STORAGE`
- [ ] Run: `go test ./internal/application/service ./internal/interfaces/http -run 'TestVFS(Mkdir|Rename|Move|Copy|Delete)' -v`
- [ ] Commit: `git commit -m "feat: add v2 fs write apis"`

### Task 8: 上传流程迁移到 `virtual_path`

**Files:**
- Modify: `backend/internal/application/dto/upload_dto.go`
- Modify: `backend/internal/application/service/upload_service.go`
- Modify: `backend/internal/interfaces/http/handler/upload_handler.go`
- Modify: `backend/internal/interfaces/http/vfs_workflow_test.go`
- Modify: `backend/internal/infrastructure/persistence/gorm/models.go`

- [ ] 为上传会话模型新增字段：
  - `TargetVirtualParentPath`
  - `ResolvedSourceID`
  - `ResolvedInnerParentPath`
- [ ] 在 upload init 逻辑中，用 `target_virtual_parent_path` 做落点解析，并把解析快照写入上传会话。
- [ ] 保持分片上传与完成上传阶段继续复用当前 local / s3 存储实现，不重新设计传输协议。
- [ ] 在 `vfs_workflow_test.go` 中新增失败测试：
  - `TestVFSUploadInitToMappedPath`
  - `TestVFSUploadInitRejectsPureVirtualParent`
- [ ] Run: `go test ./internal/interfaces/http -run 'TestVFSUpload' -v`
- [ ] Commit: `git commit -m "feat: migrate upload to virtual paths"`

### Task 9: 给 ACL / Share / Task / Trash 增加 `virtual_path` 与解析快照

**Files:**
- Modify: `backend/internal/application/dto/share_dto.go`
- Modify: `backend/internal/application/dto/task_dto.go`
- Modify: `backend/internal/application/dto/trash_dto.go`
- Modify: `backend/internal/application/dto/acl_dto.go`
- Modify: `backend/internal/application/service/acl_service.go`
- Modify: `backend/internal/application/service/acl_authorizer.go`
- Modify: `backend/internal/application/service/share_service.go`
- Modify: `backend/internal/application/service/task_service.go`
- Modify: `backend/internal/application/service/trash_service.go`
- Modify: `backend/internal/infrastructure/persistence/gorm/models.go`
- Modify: `backend/internal/interfaces/http/router_test.go`

- [ ] 先只做“模型铺底 + 最小双写”，不要一口气删除旧字段。
- [ ] ACL：新增 `virtual_path` 规则字段，并在 authorizer 中优先按虚拟路径判定；旧 `source_id + path` 只作为迁移兼容。
- [ ] Share：新增 `target_virtual_path`、`resolved_source_id`、`resolved_inner_path`。
- [ ] Task：新增 `save_virtual_path`、`resolved_source_id`、`resolved_inner_save_path`。
- [ ] Trash：新增 `original_virtual_path`。
- [ ] 增加服务层测试，确保这些新字段至少在 create / restore / get 时可写可读。
- [ ] Run: `go test ./internal/application/service ./internal/interfaces/http -run 'Test(ACL|Share|Task|Trash)' -v`
- [ ] Commit: `git commit -m "feat: add virtual path snapshots to business modules"`

### Task 10: 文档、契约与全量回归

**Files:**
- Modify: `backend/API_CONTRACT.md`
- Modify: `backend/CHANGELOG.md`
- Modify: `docs/superpowers/specs/2026-04-22-unified-storage-vfs-design.md`

- [ ] 在 `backend/API_CONTRACT.md` 中补齐 `/api/v2/fs/*` 契约、错误码、`mount_path` 字段与“同父目录名称唯一”规则。
- [ ] 在 `backend/CHANGELOG.md` 记录：
  - source 新增 `mount_path`
  - VFS 核心
  - V2 文件接口
  - 上传与业务快照字段
- [ ] 执行最小回归：
  - `go test ./internal/application/service -v`
  - `go test ./internal/interfaces/http -v`
  - `go test ./...`
- [ ] 检查是否仍有北向 `source_id` 泄漏到 V2 DTO；若有，回到对应任务修正。
- [ ] Commit: `git commit -m "docs: record vfs api and migration changes"`

---

## 自检清单

- [ ] spec 中的 `mount_path + root_path` 双路径模型在计划内有明确落地任务
- [ ] 最长前缀匹配、纯虚拟目录投影、真实/虚拟目录合并都有对应测试任务
- [ ] “虚拟路径可写但纯虚拟目录不可写”的语义在 Task 4 / Task 7 / Task 8 中有明确实现
- [ ] “同父目录名称唯一”规则在 Task 4 / Task 7 / Task 10 中有明确实现与文档落地
- [ ] 第一阶段未把 `.balance`、Alias union、统一 WebDAV 偷渡进本计划

---

## 交付顺序建议

按顺序执行，不要并行跳步：

1. Task 1-4：先把模型与 VFS 解析规则做稳
2. Task 5-7：先跑通 V2 文件系统 API
3. Task 8：接上传
4. Task 9：铺业务模块迁移基础
5. Task 10：文档与全量回归

每个 Task 完成后都应运行对应最小测试再提交，避免把错误带到后续任务。
