# Type Safety

> Type safety patterns in this project.

## Overview

The frontend uses TypeScript with build settings in `web/tsconfig.app.json`:

- `noUnusedLocals: true`
- `noUnusedParameters: true`
- `erasableSyntaxOnly: true`
- `noFallthroughCasesInSwitch: true`
- `verbatimModuleSyntax: true`
- `moduleResolution: bundler`
- Alias: `@/* -> ./src/*`

Build command:

```bash
npm run build
```

## API Type Organization

Shared API DTOs live in `web/src/types/api.ts` and mirror backend JSON fields, including snake_case names.

Envelope types:

```ts
export interface ApiResponse<T> {
  success: boolean
  code: string
  message: string
  data: T
  meta: { request_id: string; timestamp: string; pagination?: PaginationMeta }
}

export interface ApiError {
  success: false
  code: string
  message: string
  error: { details?: Record<string, unknown> }
  meta: { request_id: string; timestamp: string }
}
```

Use API modules with typed return payloads:

```ts
v2Client.get<VFSListResult>('/fs/list', { params })
apiClient.post<{ source: StorageSource }>('/sources', data)
```

## Type Placement

- Cross-page/API DTOs: `src/types/api.ts`.
- API-module-only request shapes may live in the API module, e.g. `VFSMkdirRequest` in `api/fileV2.ts`.
- Component-only props/interfaces stay in the component file.
- Use `import type` for type-only imports.

## Runtime Validation

The frontend currently does not use Zod/Yup. Backend Gin binding and typed API client assumptions are the validation boundary. Therefore:

- Keep `backend/API_CONTRACT.md`, backend DTOs, and `src/types/api.ts` in sync.
- Do not assume fields absent from current backend code.
- When docs conflict, verify current backend handler/DTO code and `backend/API_CONTRACT.md`.

## Error Types

`ApiClient` converts Axios errors into `Error` objects. Components should catch `unknown` and narrow:

```ts
catch (err: unknown) {
  const msg = err instanceof Error ? err.message : '创建失败'
  addToast(msg, 'error')
}
```

For special low-level needs, use `apiClient.getRawInstance()` with explicit `ApiResponse<T>` typing, as upload chunks do for `Blob` / `application/octet-stream`.

## Common Patterns

- Use string literal unions for finite backend values: roles, statuses, `delete_mode`, preview/download purpose.
- Use `number | null` for backend nullable numeric fields.
- Use optional properties only where backend may omit or V1/V2 fields are feature-specific.
- Keep snake_case for API DTO fields; convert only in local UI helpers if necessary.

## Forbidden Patterns

| Forbidden | Use instead |
|---|---|
| `any` for API responses | `ApiResponse<T>` / typed DTO interface |
| Blind type assertions from API payloads | Update `types/api.ts` and API module generic |
| Swallowing `unknown` errors | Narrow and show toast/inline error |
| Duplicate DTOs across pages | Centralize in `types/api.ts` or API module |
| Treating V1 and V2 file DTOs as interchangeable | Use `FileItem` vs `VFSItem` explicitly |

## Wrong vs Correct

### Wrong

```ts
const res: any = await apiClient.get('/sources')
setSources(res.items)
```

### Correct

```ts
const res = await sourceApi.list({ view: 'navigation' })
setSources(res.items)
```
