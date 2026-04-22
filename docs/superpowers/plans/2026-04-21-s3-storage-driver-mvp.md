# S3 Storage Driver MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 Yunxia 的第二个真实存储驱动可用：支持 S3 source 创建/测试、基础浏览、上传初始化与完成、下载访问地址。

**Architecture:** 继续保留当前应用层 API 不变，在应用服务下沉一个最小 `storage driver` 抽象，把 local 与 S3 的差异封装在 driver 层。P0/MVP 只做浏览、上传、下载与 access-url，不做 WebDAV over S3、缩略图、服务端 copy/move 优化。S3 直传采用预签名 multipart 方案，浏览与下载通过 SDK + 预签名 URL。

**Tech Stack:** Go 1.25, AWS SDK for Go v2, Gin, existing source/file/upload services, SQLite metadata, httptest

---

### Task 1: 建立 S3 配置模型与 source 校验

**Files:**
- Modify: `backend/internal/application/service/source_service.go`
- Modify: `backend/internal/application/dto/source_dto.go`
- Create: `backend/internal/infrastructure/storage/s3_config.go`
- Create: `backend/internal/infrastructure/storage/s3_client_factory.go`
- Test: `backend/internal/interfaces/http/storage_workflow_test.go`

- [ ] 定义 S3 config 字段：`endpoint`、`region`、`bucket`、`base_prefix`、`force_path_style`。
- [ ] 定义 secret_patch 字段：`access_key`、`secret_key`。
- [ ] `POST /sources/test` 支持 `driver_type=s3`，最小校验为 bucket 可访问。
- [ ] `POST /sources` / `PUT /sources/:id` 支持保存 S3 配置与 secret mask。
- [ ] 新增测试：创建 S3 source 返回 `driver_type=s3`，详情接口正确回显配置并掩码 secret。

### Task 2: 抽象文件浏览与下载读取能力

**Files:**
- Modify: `backend/internal/application/service/file_service.go`
- Create: `backend/internal/application/service/storage_driver.go`
- Create: `backend/internal/infrastructure/storage/local_driver.go`
- Create: `backend/internal/infrastructure/storage/s3_driver.go`
- Test: `backend/internal/application/service/service_test.go`
- Test: `backend/internal/interfaces/http/storage_workflow_test.go`

- [ ] 提炼文件服务依赖的最小能力：`List`、`SearchByName`、`Stat`、`Delete`、`PresignDownload`。
- [ ] local 改走 driver；S3 实现对象列表与按前缀搜索。
- [ ] `files/access-url` 对 S3 返回预签名下载 URL。
- [ ] `files/download` 对 S3 先走 302/代理二选一，MVP 优先 302 到预签名 URL。
- [ ] 新增测试：S3 source 下 `files/list`、`files/search`、`files/access-url` 正常返回。

### Task 3: 完成 S3 上传 MVP

**Files:**
- Modify: `backend/internal/application/service/upload_service.go`
- Modify: `backend/internal/application/dto/upload_dto.go`
- Create: `backend/internal/infrastructure/storage/s3_multipart.go`
- Modify: `backend/internal/domain/entity/upload_session.go`
- Modify: `backend/internal/infrastructure/persistence/gorm/models.go`
- Modify: `backend/internal/infrastructure/persistence/gorm/upload_session_repo_impl.go`
- Test: `backend/internal/interfaces/http/storage_workflow_test.go`

- [ ] 在上传会话中增加 `storage_data`/multipart 元信息。
- [ ] `upload/init` 在 S3 下返回 `transport.mode=direct_part_presigned` 与 `part_instructions`。
- [ ] `upload/finish` 在 S3 下消费前端上传后的 `etag parts` 并调用 complete multipart。
- [ ] 继续保留 local 的 `server_chunk` 路径不变。
- [ ] 新增测试：S3 source 下 init 返回 multipart 指令，finish 可完成并在 list 中可见。

### Task 4: 服务装配与回归验证

**Files:**
- Modify: `backend/cmd/server/main.go`
- Modify: `backend/internal/interfaces/http/router_test.go`
- Modify: `backend/go.mod`

- [ ] 在主程序中装配 storage driver factory。
- [ ] 在测试路由中提供 fake S3 backend 或 minio/httptest stub。
- [ ] Run: `go test ./...`

---

## Self-review

- 该计划刻意把 S3 限定为 MVP，不引入 ACL、WebDAV over S3、对象元数据高级能力。
- local 现有能力必须保持回归通过。
- 若执行时发现 service 继续堆叠 `if driverType == ...`，应优先把 driver 抽象落稳，再继续扩能力。
