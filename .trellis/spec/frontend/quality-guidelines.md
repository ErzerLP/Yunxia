# Quality Guidelines

> Code quality standards for frontend development.

## Overview

Frontend quality is enforced by TypeScript build, ESLint, and project-specific static regression checks.

Run from `web/`:

```bash
npm run lint
npm run build
node scripts/check-vfs-integration.mjs
```

`npm run build` runs `tsc -b && vite build`.

## Required Patterns

### API contract discipline

- Use `apiClient` / `v2Client`, not raw `fetch`, for JSON REST APIs.
- Check `backend/FRONTEND_HANDOFF.md` first when implementing a backend-to-frontend
  adaptation item; use its top `待适配索引` to find the active row, then read the
  linked detail checklist.
- Keep `src/types/api.ts` synced with `backend/API_CONTRACT.md` and backend DTOs.
- Treat `backend/API_CONTRACT.md` as the route/DTO/error/capability truth source;
  treat `backend/FRONTEND_HANDOFF.md` as the work queue and completion checklist.
- Treat `web/FRONTEND_TEST_HANDOFF.md` as the tester-facing integration/regression
  queue for frontend changes that need focused QA or runtime smoke coverage.
- VFS uses `/api/v2/fs*` through `fileV2Api`.

### Frontend handoff completion discipline

When frontend work adapts any item from `backend/FRONTEND_HANDOFF.md`, the
implementation is not complete until the handoff document is updated in the
same change:

- Tick only the checklist items actually implemented and verified.
- Keep the top `待适配索引` row status and the detail title status in sync.
- Use only the fixed status values from the handoff file: `待适配`, `适配中`,
  `待联调`, `已适配`, `暂缓`, `废弃`.
- Mark `待联调` when frontend code and static checks pass but an end-to-end
  backend/runtime smoke test has not been completed.
- Mark `已适配` only after the frontend implementation is complete and the
  relevant verification/smoke result is recorded.
- Add a compact verification note under the detail entry when marking
  `待联调` or `已适配`, for example frontend commands run, smoke path, and any
  known limitation.
- Never delete historical handoff details; update status/checklist/notes in
  place.

### Frontend test handoff discipline

When frontend changes affect user-visible flows, permissions, API integration,
tasks, files/VFS behavior, or any regression area that QA should prioritize,
update `web/FRONTEND_TEST_HANDOFF.md` in the same change:

- Maintain both the top `待测试索引` row and the linked detail entry.
- Use only the fixed status values from that file: `待联调`, `联调中`, `待回归`,
  `阻塞`, `已通过`, `暂缓`, `废弃`.
- Each index row must include status, date, module, affected pages, priority,
  key interfaces, testing focus, and a stable detail anchor.
- Each detail entry must include checklist, prerequisites, test steps, expected
  results, regression scope, blockers/notes, and handoff records.
- Mark `已通过` only after the runtime integration/regression result is recorded.
- Never delete historical test handoff details; update status/checklist/notes in
  place.

### Auth and capability discipline

- `App.tsx` checks setup status and auth state globally.
- Public share route `/s/:token` must stay unauthenticated and must not be proxied by Vite.
- Protected pages need both sidebar filtering and `CapabilityRoute`.
- Button-level actions should use capabilities/current directory permissions where relevant.
- Pages that are visible with one capability but enrich data from a stricter
  capability must gate the stricter query separately. Example: `/sources` is
  visible with `source.read`, but `/api/v1/system/config` requires
  `system.config.read`; operator users must not trigger a 403 just to render
  WebDAV status, and unknown global WebDAV status must not be displayed as
  "disabled".

### File/VFS regression discipline

`web/scripts/check-vfs-integration.mjs` documents important invariants. Preserve these unless a task explicitly changes the contract:

- `/files` renders `VFSFileManagerPage`.
- VFS downloads/previews call `fileV2Api.accessUrl` before opening.
- VFS search uses `path`, not `path_prefix`.
- Upload in VFS mode sends `target_virtual_parent_path`.
- File lists render `displayedFiles` / `displayedVfsItems` to avoid transient empty states.
- File query caches are invalidated/refetched after upload/task completion.

## Forbidden Patterns

| Forbidden | Why |
|---|---|
| Raw JSON fetches scattered in components | Bypasses auth refresh/error unwrapping |
| Empty catches on user actions | Users see no failure reason |
| Direct `window.open(fileV2Api.download(path))` for VFS | Missing temporary access token |
| Adding a `/s/` Vite proxy | Breaks React public share route rendering |
| Reintroducing separate VFS sidebar item | `/files` is unified file entry |
| Showing raw DB/UNIQUE constraint text in UI | Bad UX and leaks internals |
| Mutations without query invalidation | Stale lists after task/upload/share changes |
| Adapting a `backend/FRONTEND_HANDOFF.md` item without updating its index status and checklist | Future frontend work cannot tell what is really done |
| Marking a handoff item `已适配` without verification evidence | Makes untested integration look complete |
| Shipping tester-visible frontend changes without updating `web/FRONTEND_TEST_HANDOFF.md` | QA cannot find the focused integration/regression scope |

Exception: direct object upload/download URLs that are not Yunxia JSON REST
APIs may use browser primitives. Existing `UploadModal` uses `fetch` for
pre-signed chunk upload instructions; keep JSON API calls on `apiClient` /
`v2Client`.

## Testing / Verification Requirements

There are currently no frontend unit test files. For changes, use:

1. `npm run lint`
2. `npm run build`
3. `node scripts/check-vfs-integration.mjs` when touching files/VFS/share/upload/task/auth/setup routes

For handoff-driven features, also verify the documentation state:

- The handoff top index row points to an existing detail anchor.
- The top index status, detail title status, and checked checklist items match
  the actual frontend implementation.
- Any `待联调` / `已适配` status has a short verification note.
- If the change needs QA focus, `web/FRONTEND_TEST_HANDOFF.md` has a matching
  top index row and detail entry with prerequisites, steps, expected results,
  regression scope, and blockers/notes.

For UI changes, manually verify in Vite dev server when feasible:

```bash
npm run dev
```

## Accessibility Checklist

- [ ] Labels use `htmlFor` and inputs have matching `id`.
- [ ] Inputs have `name` when used in forms.
- [ ] Password fields use correct `autoComplete`.
- [ ] Icon-only buttons have `title` or accessible text.
- [ ] Loading/disabled states are visible and block duplicate submits.
- [ ] Errors are visible inline and/or via toast.

## Code Review Checklist

- [ ] `npm run lint` passes.
- [ ] `npm run build` passes.
- [ ] Static VFS integration checks pass when relevant.
- [ ] Adapted `backend/FRONTEND_HANDOFF.md` items have synchronized index
      status, detail title status, checklist ticks, and verification notes.
- [ ] Tester-visible integration/regression work has a synchronized
      `web/FRONTEND_TEST_HANDOFF.md` index row and detail entry.
- [ ] Query keys and invalidations cover affected views.
- [ ] Capability guards match backend capability names.
- [ ] API clients return typed `data`, not envelopes, unless raw instance is explicitly needed.
- [ ] UI copy and styling match existing Chinese app tone and Tailwind token system.

## Scenario: Backend-to-Frontend Handoff Consumption and Completion

### 1. Scope / Trigger

- Trigger: frontend adapts any feature/interface listed in
  `backend/FRONTEND_HANDOFF.md`, or adds/changes API clients, DTO types, routes,
  pages, permissions, or UI flows because backend exposed new frontend-visible
  behavior.
- Goal: keep implementation, API truth, and the handoff work queue searchable
  and synchronized.

### 2. Signatures

- Handoff file: `backend/FRONTEND_HANDOFF.md`.
- API truth file: `backend/API_CONTRACT.md`.
- Handoff index row shape:

```text
| 状态 | 日期 | 模块 | 影响页面 | 优先级 | 关键接口 | 详情 |
```

- Detail anchor:

```html
<a id="handoff-YYYY-MM-DD-feature"></a>
```

- Detail title:

```text
### [P1][待适配][模块] YYYY-MM-DD 标题
```

- Frontend implementation locations usually include:
  - `web/src/api/<domain>.ts`
  - `web/src/types/api.ts`
  - `web/src/pages/<domain>/<Name>Page.tsx`
  - `web/src/router/index.tsx`
  - `web/src/components/layout/Sidebar.tsx`

### 3. Contracts

- Read order for handoff work:
  1. `backend/FRONTEND_HANDOFF.md` top `待适配索引`
  2. linked handoff detail checklist
  3. `backend/API_CONTRACT.md` for exact routes, fields, errors, permissions
  4. current backend handler/DTO code only when docs and runtime evidence conflict
- Status values are fixed: `待适配`, `适配中`, `待联调`, `已适配`, `暂缓`, `废弃`.
- If a status changes, update both the top index row and the detail title.
- If only part of the checklist is implemented, leave remaining items unchecked
  and do not mark the whole entry `已适配`.
- Use `待联调` for frontend-complete work that still needs backend/runtime smoke
  verification.
- Use `已适配` only when the feature is implemented, relevant quality checks pass,
  and verification is recorded in the detail entry.
- API route/DTO/error/capability changes belong in `backend/API_CONTRACT.md`;
  frontend completion state belongs in `backend/FRONTEND_HANDOFF.md`.

### 4. Validation & Error Matrix

| Condition | Required action |
|---|---|
| Handoff row exists and frontend adapts it | Update checklist, status, and verification note in the same change |
| Frontend implementation is complete but no runtime smoke was done | Mark `待联调`, not `已适配` |
| Runtime/integration smoke passed | Mark `已适配` and record the verification path/result |
| API_CONTRACT and handoff details disagree | Verify backend handler/DTO behavior before coding; document the resolved contract |
| Backend exposes a frontend-visible change but no handoff row exists | Add/update a row in `backend/FRONTEND_HANDOFF.md` instead of creating a one-off note |
| Feature is blocked by backend/runtime/config | Mark `暂缓` or leave `待联调` with a concise blocking note |

### 5. Good/Base/Bad Cases

- Good: RSS adaptation adds `rssApi`, DTOs, route, sidebar entry, capability
  gates, and page behavior; then ticks implemented checklist items, updates the
  status to `待联调` or `已适配`, and records `npm run lint`, `npm run build`,
  plus smoke results.
- Base: a small DTO field added for an existing page updates `types/api.ts`, the
  affected UI, and the matching handoff checklist item.
- Bad: shipping frontend code for a handoff entry while the top index still says
  `待适配` and every checklist item remains unchecked.

### 6. Tests Required

- Static checks: `npm run lint` and `npm run build`.
- Domain checks: run `node scripts/check-vfs-integration.mjs` when the change
  touches files/VFS/share/upload/task/auth/setup routes.
- Runtime smoke when feasible: exercise the page flow described by the handoff
  detail against the backend and record the result before marking `已适配`.
- Documentation consistency: verify the index link resolves to the detail
  anchor, status values match, and checked checklist items correspond to code.
- Diff hygiene: run `git diff --check` for spec/doc-heavy changes.

### 7. Wrong vs Correct

#### Wrong

```text
Implement RSS page and API client, but leave backend/FRONTEND_HANDOFF.md showing:
| 待适配 | 2026-04-29 | RSS 订阅 | ... |
and no checked checklist items or verification note.
```

#### Correct

```text
Implement RSS page and API client;
update backend/FRONTEND_HANDOFF.md top index and detail title to 待联调;
tick implemented checklist items;
append verification notes for lint/build and the remaining smoke gap.
```

## Scenario: Cloud Storage / Source Driver Frontend Adaptation

### 1. Scope / Trigger

- Trigger: backend exposes or changes a storage driver such as `s3` or `pikpak`,
  changes upload transport, WebDAV source exposure, VFS write semantics, or task
  downloader types.
- Goal: adapt the frontend without hard-coding local-only assumptions or leaking
  secret/backend internals.

### 2. Signatures

- Source upsert: `POST/PUT /api/v1/sources*` with `driver_type`,
  `config`, `secret_patch`, `is_webdav_exposed`, `webdav_read_only`,
  `mount_path`, and `root_path`.
- Source detail: `GET /api/v1/sources/:id` returns
  `{source, config, secret_fields, last_checked_at}`.
- Upload init: `POST /api/v1/upload/init` returns `transport.mode` as
  `server_chunk` or `direct_parts`.
- Tasks: `DownloadTask.downloader_type` can grow beyond local downloader names;
  current known values include `aria2`, `qbittorrent`, and `pikpak_native`.
- VFS write APIs: use `fileV2Api` and the backend-provided capability/ACL result,
  especially `current_permissions` and item-level `can_delete`.

### 3. Contracts

- Keep API DTO fields in snake_case and mirror `backend/API_CONTRACT.md`.
- Secret editing must be patch-based:
  - omitted secret field means "keep existing"
  - `null` means "clear this secret"
  - non-empty string means "replace this secret"
- Secret display must use `secret_fields` masks unless the user has
  `source.secret.read`; never infer plaintext availability from driver type.
- Non-local sources can be WebDAV-exposed when backend supports it. The frontend
  only shows/copies the URL and read-only state; it does not implement a WebDAV
  client.
- Upload direct URLs/headers are short-lived transport instructions. Do not log,
  persist, or store `Authorization`, `X-OSS-Security-Token`, or provider-specific
  signing headers.

### 4. Validation & Error Matrix

| Condition | Required frontend behavior |
|---|---|
| `CLOUD_AUTH_FAILED` / `CLOUD_TOKEN_INVALID` | Prompt the admin to check account/password/token |
| `CLOUD_CAPTCHA_REQUIRED` | Explain captcha verification and token refill |
| `CLOUD_RATE_LIMITED` | Ask user to retry later |
| `CLOUD_PROVIDER_UNAVAILABLE` | Show provider temporarily unavailable |
| `SOURCE_OPERATION_UNSUPPORTED` | Explain the unsupported operation, e.g. pause/resume/permanent delete |
| `FILE_ALREADY_EXISTS` / `NAME_CONFLICT` | Tell the user to rename or choose another target |
| `NO_BACKING_STORAGE` | Tell the user to enter a concrete mounted directory |

### 5. Good/Base/Bad Cases

- Good: PikPak create/edit form exposes public config, secret masks, `null`
  clearing, WebDAV toggles, and VFS/task/upload flows all work through existing
  API clients and shared error mapping.
- Base: adding a new enum value only updates DTOs, labels, and unsupported-action
  hints.
- Bad: hiding all write actions for a driver because it is not `local`, submitting
  empty secret strings on every edit, or directly showing raw database/provider
  errors.

### 6. Tests Required

- Static: `npm run lint`, `npm run build`, and
  `node scripts/check-vfs-integration.mjs` when touching VFS/upload/task/source
  flows.
- Handoff: update `backend/FRONTEND_HANDOFF.md` status/checklist/verification and
  `web/FRONTEND_TEST_HANDOFF.md` tester-facing steps.
- Runtime smoke when feasible:
  - create/edit/test the source
  - verify masked/plain secret display by capability
  - perform VFS mkdir/rename/move/copy/delete
  - upload through both supported `transport.mode` branches if fixtures allow
  - create/cancel tasks and verify downloader labels/action availability
  - copy WebDAV URL and smoke read/write behavior for the slug

### 7. Wrong vs Correct

#### Wrong

```tsx
await sourceApi.update(id, {
  config,
  secret_patch: {
    username: '',
    password: '',
    refresh_token: '',
  },
  is_webdav_exposed: source.driver_type === 'local' && isWebDAVExposed,
})
```

#### Correct

```tsx
const secret_patch = buildSecretPatchFromTouchedFields()
await sourceApi.update(id, {
  config,
  ...(Object.keys(secret_patch).length > 0 ? { secret_patch } : {}),
  is_webdav_exposed: isWebDAVExposed,
  webdav_read_only: webDAVReadOnly,
})
```

## Wrong vs Correct

### Wrong

```ts
try {
  await sourceApi.create(payload)
} catch {}
```

### Correct

```ts
try {
  await sourceApi.create(payload)
  addToast('存储源创建成功', 'success')
  queryClient.invalidateQueries({ queryKey: ['sources'] })
} catch (err: unknown) {
  const message = getCreateSourceErrorMessage(err)
  setCreateError(message)
  addToast(message, 'error')
}
```
