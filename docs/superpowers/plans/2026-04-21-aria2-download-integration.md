# Aria2 Download Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把离线下载从“当前最小可用”推进到“真实 Aria2 集成可联调”，补齐暂停/恢复、状态映射和服务启动接线。

**Architecture:** 保持现有 `TaskService -> Downloader interface -> infrastructure/downloader/Aria2Client` 分层，HTTP 层只暴露任务接口，应用层负责状态与错误语义，基础设施层负责 JSON-RPC 协议细节与响应解析。先在本地测试中通过 fake downloader 跑通 pause/resume，再用独立 downloader 单测验证 RPC 方法与状态映射。

**Tech Stack:** Go 1.25, Gin, existing task repo/service/handler, Aria2 JSON-RPC, net/http/httptest, standard testing

---

### Task 1: 补齐 downloader 能力边界

**Files:**
- Modify: `backend/internal/application/service/task_service.go`
- Modify: `backend/internal/infrastructure/downloader/aria2_client.go`
- Test: `backend/internal/infrastructure/downloader/aria2_client_test.go`

- [ ] 定义 `Downloader` 新方法：`Pause(ctx, externalID)`、`Resume(ctx, externalID)`。
- [ ] 为 `Aria2Client` 增加 `aria2.pause`、`aria2.unpause` 调用实现。
- [ ] 新增单测：验证 `Pause` 调用 `aria2.pause`，`Resume` 调用 `aria2.unpause`。
- [ ] Run: `go test ./internal/infrastructure/downloader -v`

### Task 2: 扩展任务服务与 HTTP 路由

**Files:**
- Modify: `backend/internal/application/dto/task_dto.go`
- Modify: `backend/internal/application/service/task_service.go`
- Modify: `backend/internal/interfaces/http/handler/task_handler.go`
- Modify: `backend/internal/interfaces/http/router.go`
- Test: `backend/internal/interfaces/http/task_webdav_test.go`
- Test: `backend/internal/interfaces/http/router_test.go`

- [ ] 在 DTO 中补充 `TaskActionResponse`，用于 pause/resume 响应。
- [ ] 在 `TaskService` 中实现 `Pause(id)` 与 `Resume(id)`，并做状态校验：
  - 可暂停：`pending | running`
  - 可恢复：`paused`
- [ ] 在 `TaskHandler` 中新增 `Pause` / `Resume`。
- [ ] 在 router 注册：
  - `POST /api/v1/tasks/:id/pause`
  - `POST /api/v1/tasks/:id/resume`
- [ ] 为现有 `fakeDownloader` 增加 pause/resume 状态变化。
- [ ] 新增 HTTP 测试：创建任务 -> pause -> resume -> 查询状态。
- [ ] Run: `go test ./internal/interfaces/http -run TestTaskLifecycle -v`

### Task 3: 启动接线与错误语义收口

**Files:**
- Modify: `backend/cmd/server/main.go`
- Modify: `backend/internal/interfaces/http/handler/task_handler.go`
- Modify: `backend/internal/application/service/storage_errors.go`

- [ ] 保持 `cmd/server` 默认注入真实 `Aria2Client`。
- [ ] 为 downloader 不可用、任务状态非法添加更明确错误码映射：
  - `DOWNLOADER_UNAVAILABLE`
  - `TASK_INVALID_STATE`
- [ ] 确保 pause/resume/delete 在 downloader 为空时返回统一错误。
- [ ] Run: `go test ./...`

---

## Self-review

- 当前计划只覆盖离线下载增强，不混入 S3。
- 与现有代码一致复用 `TaskService`/`TaskHandler`/`Aria2Client`，避免引入第二套任务实现。
- 验证以 downloader 单测 + HTTP 流程测为主。
