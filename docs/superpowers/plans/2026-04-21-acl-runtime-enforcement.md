# ACL Runtime Enforcement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让已可配置的 ACL 规则真正接入文件访问主链路，先覆盖 files / upload / WebDAV 的最小可生效权限判定。

**Architecture:** 新增独立 `ACLAuthorizer` 负责 multi-user 开关判断、admin bypass、路径匹配、优先级决策与 allow/deny 输出。HTTP 鉴权中间件把当前用户信息写入 request context，`FileService`、`UploadService`、`WebDAVHandler` 只依赖 authorizer 做运行时判定，不把 ACL 规则逻辑散落到各 handler。列表/搜索按结果逐项过滤；读写类接口按动作映射到 `read / write / delete`。

**Tech Stack:** Go 1.25、Gin、GORM、SQLite、httptest

---

### Task 1: 先补 ACL 运行时失败测试

**Files:**
- Modify: `backend/internal/interfaces/http/storage_workflow_test.go`
- Modify: `backend/internal/interfaces/http/task_webdav_test.go`

- [ ] 新增 local 文件主链路 ACL 集成测试：list / search / access-url / download / mkdir / rename / move / copy / delete / upload init。
- [ ] 新增 multi-user + normal user + ACL rule 的测试装配 helper。
- [ ] 新增 WebDAV 最小 ACL 集成测试，先验证普通用户对允许/拒绝路径的读写行为。
- [ ] 运行针对性测试并确认在实现前失败。

### Task 2: 实现 ACLAuthorizer 与 request auth 透传

**Files:**
- Create: `backend/internal/infrastructure/security/request_auth.go`
- Create: `backend/internal/application/service/acl_authorizer.go`
- Modify: `backend/internal/application/service/storage_errors.go`
- Modify: `backend/internal/interfaces/middleware/auth_middleware.go`

- [ ] 定义 request context 内的最小用户身份载体。
- [ ] 实现 authorizer：admin bypass、single-user bypass、default deny、priority desc + id asc、inherit_to_children。
- [ ] 把 `ACL_DENIED` 对应的领域错误补到应用层。
- [ ] 让 Bearer 鉴权成功后把用户信息写入 request context。

### Task 3: 把 ACL 接入 FileService / UploadService / WebDAV

**Files:**
- Modify: `backend/internal/application/service/storage_driver.go`
- Modify: `backend/internal/application/service/file_service.go`
- Modify: `backend/internal/application/service/upload_service.go`
- Modify: `backend/internal/interfaces/http/handler/file_handler.go`
- Modify: `backend/internal/interfaces/http/handler/upload_handler.go`
- Modify: `backend/internal/interfaces/http/handler/webdav_handler.go`

- [ ] 文件列表 / 搜索按 item 逐条做 `read` 过滤。
- [ ] access-url / download 做 `read` 判定。
- [ ] mkdir / rename / move / copy / delete / upload init 做最小写权限判定。
- [ ] WebDAV 按方法映射 `read / write / delete` 并做最小运行时拦截。
- [ ] handler 层补 `ACL_DENIED -> 403` 错误映射。

### Task 4: 装配、回归与记录

**Files:**
- Modify: `backend/cmd/server/main.go`
- Modify: `backend/internal/interfaces/http/router_test.go`
- Modify: `backend/CHANGELOG.md`

- [ ] 装配共享 `ACLAuthorizer` 到 file / upload / webdav。
- [ ] Run: `go test ./internal/interfaces/http -run 'Test(LocalFileACL|WebDAV)' -v`
- [ ] Run: `go test ./...`
- [ ] 更新 changelog，记录 ACL 已从“可配置”推进到“运行时生效”。
