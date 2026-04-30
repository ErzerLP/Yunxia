# Quality Guidelines

> Code quality standards for backend development.

## Overview

Backend quality means:

1. Preserve DDD layer boundaries and dependency inversion.
2. Preserve API/error/audit contracts used by the frontend.
3. Prove behavior with focused Go tests.
4. Leave enough frontend-facing documentation for another developer to integrate without reading backend code.

Run from `backend/`:

```bash
gofmt -w <changed go files>
go test ./...
```

## Required Patterns

### Context-first APIs

Repository/service methods that do I/O should accept `context.Context`:

```go
FindByID(ctx context.Context, id uint) (*entity.User, error)
```

### Dependency injection

Concrete dependencies are wired in `cmd/server/main.go`; services receive repositories/interfaces/drivers via constructors/options.

### Capability-aware routes

Protected management routes use middleware and constants from `internal/domain/permission`:

```go
configWrite := authorized.Group("")
configWrite.Use(middleware.RequireCapability(permission.CapabilitySystemConfigWrite))
configWrite.PUT("/system/config", systemHandler.UpdateConfig)
```

### VFS and path safety

Virtual paths must be normalized with service helpers. User paths are absolute virtual paths beginning with `/`; write targets must be checked for mount/pure-virtual/name conflicts where relevant.

### Audit for mutations

User/source/ACL/file/upload/task/share/system mutations should record best-effort audit events when the service has an audit recorder option pattern.

### Frontend integration handoff

Any backend change that adds or changes frontend-facing modules, routes, DTO
fields, capabilities, error codes, or user flows must include a frontend
handoff note before it is considered complete.

Minimum requirement:

- Update `backend/API_CONTRACT.md` in the same change.
- Document route method/path, auth/capability requirement, request payload,
  response shape, stable error codes, and important state transitions.
- Include at least one request/response example when the flow is new,
  multi-step, or not obvious from the route table.
- Call out frontend pitfalls: wrapper shape (`{items}` vs direct object),
  redirect/download behavior, nullable fields, polling expectations, and
  permission-gated visibility.

For a new feature page or a multi-endpoint module, add a short feature section
to `backend/API_CONTRACT.md` that explains the recommended page flow, not only
the raw endpoint list.

Do not create a new handoff document for every backend update. Use the fixed
backend handoff file `backend/FRONTEND_HANDOFF.md` for frontend integration
notes. The file must keep a top `待适配索引` table plus detailed entries at the
end. Every new handoff entry must:

- Add or update one row in `待适配索引`.
- Use a stable detail anchor, e.g. `<a id="handoff-YYYY-MM-DD-feature"></a>`.
- Use a title shaped like `[P1][待适配][模块] YYYY-MM-DD 标题`.
- Include a `前端适配 checklist`.
- Use only fixed status values: `待适配` / `适配中` / `待联调` / `已适配` / `暂缓` / `废弃`.

Append detailed updates at the end with date/feature headings. Link or reference
this fixed file from `backend/API_CONTRACT.md` when the API contract alone is
not enough.

When creating any new backend-owned or project coordination document, check
whether an index document must be updated in the same change. Usually this means
updating `DOCS-INDEX.md`, `docs/README.md`, `backend/API_CONTRACT.md`, or the
relevant fixed handoff/runbook index so the new document has a discoverable
owner, purpose, and reading path. This keeps documentation ordered and avoids
scattered one-off docs.

## Scenario: Frontend Handoff and Documentation Index Maintenance

### 1. Scope / Trigger

- Trigger: any backend change that adds or changes frontend-visible API routes,
  DTO fields, permissions, error codes, user flows, deployment behavior, or a
  new coordination document.
- Goal: keep API truth, frontend TODOs, and document entry points searchable
  without creating scattered one-off files.

### 2. Signatures

- API contract file: `backend/API_CONTRACT.md`.
- Fixed handoff file: `backend/FRONTEND_HANDOFF.md`.
- Handoff index row:

```text
| 状态 | 日期 | 模块 | 影响页面 | 优先级 | 关键接口 | 详情 |
```

- Handoff detail anchor:

```html
<a id="handoff-YYYY-MM-DD-feature"></a>
```

- Handoff detail title:

```text
### [P1][待适配][模块] YYYY-MM-DD 标题
```

### 3. Contracts

- `状态` must be one of: `待适配`, `适配中`, `待联调`, `已适配`, `暂缓`, `废弃`.
- `详情` must link to a stable in-file anchor.
- Every new detail entry must include `前端适配 checklist`.
- API route, payload, response shape, permission, and stable error code details
  belong in `backend/API_CONTRACT.md`; the handoff file should summarize what
  the frontend must change and link back to the API contract.
- If a new document is created, update the relevant index in the same change
  when the document needs to be discoverable (`DOCS-INDEX.md`, `docs/README.md`,
  a backend fixed handoff/runbook index, or a package spec index).

### 4. Validation & Error Matrix

| Condition | Required action |
|---|---|
| New frontend-visible route/DTO/error/capability | Update `backend/API_CONTRACT.md` |
| Frontend needs UI/API-client changes | Add/update `backend/FRONTEND_HANDOFF.md` index row and detail checklist |
| Existing handoff item changes status | Update both top index row and detail title/checklist |
| New standalone doc added | Check and update the relevant index document |
| Handoff index grows too large | Split index into in-file status/module subsections; do not create scattered handoff docs |

### 5. Good/Base/Bad Cases

- Good: RSS module adds API contract section, a single indexed handoff entry,
  stable anchor, checklist, permissions, errors, and page flow.
- Base: small DTO field addition updates API contract and appends one checklist
  item to an existing handoff entry.
- Bad: adding `docs/rss-frontend-todo-final-v2.md` without linking it from an
  index or updating the fixed handoff file.

### 6. Tests Required

- Documentation check: verify route names, JSON fields, capability names, and
  error codes match current handlers/DTOs.
- Searchability check: from `backend/FRONTEND_HANDOFF.md` top index, every row's
  `详情` link resolves to a detail anchor.
- Consistency check: when a status changes, the top index, detail title, and
  checklist all use the same status.
- Quality gate: run `git diff --check`; run backend tests when code changed.

### 7. Wrong vs Correct

#### Wrong

```text
新增 backend/rss-note.md，写“前端自己看接口”，但不更新 API_CONTRACT / FRONTEND_HANDOFF / DOCS-INDEX。
```

#### Correct

```text
更新 backend/API_CONTRACT.md 的 RSS 路由契约；
在 backend/FRONTEND_HANDOFF.md 顶部索引新增一行并链接到 stable anchor；
在同一详情下追加 checklist、页面流程、错误处理和权限说明。
```

## Forbidden Patterns

| Forbidden | Use instead |
|---|---|
| Handler imports GORM or touches DB | Call application service |
| Service imports Gin or writes HTTP responses | Return DTO + error |
| Domain imports infra/framework packages | Define interfaces in domain, implement outside |
| Compare error strings | `errors.Is` sentinel checks |
| Raw local filesystem path joins | Existing normalize/resolve helpers |
| New route without `backend/API_CONTRACT.md` update | Update contract with method/path/payload/errors |
| Frontend-facing module without handoff index/checklist | Update API contract and `backend/FRONTEND_HANDOFF.md` index + checklist |
| Silent ignored errors in background/mutation paths | Log or audit best-effort failure |

Current application code has narrow imports of infrastructure helpers for auth
context, logging context, and S3 config parsing. Reuse those only when matching
the existing pattern; for new dependency types prefer a domain/application
interface wired from `cmd/server/main.go` instead of expanding service-layer
infrastructure coupling.

## Testing Requirements

- Service tests: `backend/internal/application/service/*_test.go`
- Repository tests: `backend/internal/infrastructure/persistence/gorm/*_test.go`
- HTTP workflow tests: `backend/internal/interfaces/http/*_test.go`

Use `t.TempDir()` for filesystem-backed behavior and existing `openTestDB(t)` helper style where available.

## Code Review Checklist

- [ ] Domain stays framework-free; application/infrastructure imports match the current seams documented in `directory-structure.md`.
- [ ] New DTO fields use snake_case JSON tags and are mirrored in frontend types if consumed.
- [ ] New routes are registered in `router.go` and documented in `backend/API_CONTRACT.md`.
- [ ] Frontend-facing changes include handoff notes: purpose, routes, payloads, response shape, errors, permissions, and recommended UI flow.
- [ ] Known user-correctable failures map to stable error codes.
- [ ] Mutations that should be auditable call the best-effort audit path.
- [ ] Repositories use `WithContext(ctx)` and explicit model/entity conversion.
- [ ] Path operations preserve VFS/local storage safety.
- [ ] `go test ./...` passes.

## Wrong vs Correct

### Wrong

```go
authorized.POST("/sources", sourceHandler.Create) // no management capability guard
```

### Correct

```go
sourceCreate := authorized.Group("")
sourceCreate.Use(middleware.RequireCapabilitiesForAction(
    auditRecorder, rootLogger, "storage_source", "create", permission.CapabilitySourceCreate,
))
sourceCreate.POST("/sources", sourceHandler.Create)
```




