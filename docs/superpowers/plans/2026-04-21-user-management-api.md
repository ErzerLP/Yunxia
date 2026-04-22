# User Management API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Yunxia 后端补齐 Draft 用户管理接口，支持管理员对用户执行 list / create / update / reset-password / revoke-tokens。

**Architecture:** 基于现有 `User` 实体、`UserRepository`、JWT token version 机制扩展一层 `UserService` 和 `UserHandler`。内部继续使用已有实体字段：`Role` 维持 `admin/user`，对外契约层把 `user` 映射为 `normal`；状态使用 `IsLocked` 映射成 `active/locked`；`last_login_at` 先返回 `null`，不为 Draft 阶段引入登录审计表。

**Tech Stack:** Go 1.25、Gin、GORM、SQLite、JWT token version、httptest

---

### Task 1: 扩展用户仓储最小能力

**Files:**
- Modify: `backend/internal/domain/repository/user_repo.go`
- Modify: `backend/internal/infrastructure/persistence/gorm/user_repo_impl.go`

- [ ] 增加 `List` 与 `Update` 最小仓储能力。
- [ ] `List` 支持 `keyword` 与 `status(active/locked)` 过滤。
- [ ] `Update` 支持更新 `email / role / is_locked / password_hash / token_version`。

### Task 2: 先写失败测试，再实现 UserService

**Files:**
- Create: `backend/internal/application/dto/user_admin_dto.go`
- Create: `backend/internal/application/service/user_service.go`
- Modify: `backend/internal/application/service/service_test.go`

- [ ] 新增服务层失败测试：list / create / update / reset-password / revoke-tokens。
- [ ] 新增最小错误：用户名冲突、角色非法、状态非法。
- [ ] 复用已有 `passwordHasher` 与 `UserRepository.UpdateTokenVersion` 语义，保持 revoke token 为 token version +1。

### Task 3: 先写失败测试，再实现 HTTP handler 与路由

**Files:**
- Create: `backend/internal/interfaces/http/handler/user_handler.go`
- Modify: `backend/internal/interfaces/http/router.go`
- Modify: `backend/internal/interfaces/http/router_test.go`

- [ ] 新增 HTTP 失败测试：`GET/POST/PUT/POST reset-password/POST revoke-tokens`。
- [ ] 路由全部挂到 `adminOnly`。
- [ ] 错误码最小覆盖：`USER_NOT_FOUND`、`USER_NAME_CONFLICT`、`USER_ROLE_INVALID`、`USER_STATUS_INVALID`。

### Task 4: 装配与回归

**Files:**
- Modify: `backend/cmd/server/main.go`
- Modify: `backend/CHANGELOG.md`

- [ ] 在主程序与测试装配里注入 `UserService` / `UserHandler`。
- [ ] Run: `go test ./internal/application/service ./internal/interfaces/http -run 'Test(UserService|UserManagement)' -v`
- [ ] Run: `go test ./...`
