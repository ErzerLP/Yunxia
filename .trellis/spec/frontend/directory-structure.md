# Directory Structure

> How frontend code is organized in this project.

## Overview

The frontend is a Vite React app under `web/`. Imports use the `@/*` alias configured in `vite.config.ts` and `tsconfig.app.json`.

Main entrypoints:

- `src/main.tsx`: React root, `QueryClient`, `RouterProvider`
- `src/router/index.tsx`: route definitions
- `src/App.tsx`: global setup/auth initialization and global upload modal
- `src/components/layout/AppLayout.tsx`: sidebar, preview drawer, toast container

## Directory Layout

```text
web/src/
├── api/              # Axios client and feature API modules
├── assets/           # Static imported assets
├── components/
│   ├── common/       # Shared business components, e.g. capability guards
│   ├── files/        # File/VFS/upload/share/file operation components
│   ├── layout/       # App shell, sidebar, preview drawer
│   └── ui/           # Generic UI primitives
├── hooks/            # Custom hooks, auth/capability guards
├── pages/            # Route-level pages grouped by domain
├── router/           # React Router definitions and guards
├── stores/           # Zustand client-state stores
├── types/            # Shared TypeScript DTO/API types
└── utils/            # Pure helpers: cn, format, VFS/WebDAV helpers
```

## Module Organization

### API modules

- One file per backend domain: `auth.ts`, `source.ts`, `file.ts`, `fileV2.ts`, `upload.ts`, `share.ts`, etc.
- Use `apiClient` for `/api/v1` and `v2Client` for `/api/v2`.
- API functions should unwrap typed `data` payloads through `ApiClient` methods.
- Current feature code imports API modules directly, e.g. `@/api/source` and
  `@/api/fileV2`. `src/api/index.ts` is only a convenience barrel for selected
  modules and is not the canonical import path.

Example:

```ts
export const sourceApi = {
  list: (params?: PaginationParams & { view?: 'navigation' | 'admin' }) =>
    apiClient.get<{ items: StorageSource[]; view: string }>('/sources', { params }),
}
```

### Pages and routes

- Pages live under `src/pages/<domain>/<Name>Page.tsx`.
- `/files` is the canonical file entry and currently renders `VFSFileManagerPage`.
- `/vfs` redirects to `/files`; do not reintroduce a separate sidebar VFS entry.
- Legacy `FileManagerPage` still exists and must set file store mode to `v1` when used.

## Naming Conventions

- Component files: PascalCase (`VFSFileList.tsx`, `PreviewDrawer.tsx`).
- API modules/stores: camelCase feature names (`fileV2.ts`, `authStore.ts`).
- Hooks start with `use` (`useAuth.ts`, `useCapability.ts`).
- Shared DTOs live in `types/api.ts` unless tightly local to one API module/component.

## Wrong vs Correct

### Wrong

```ts
import { apiClient } from '../../../api/client'
await fetch('/api/v2/fs/list')
```

### Correct

```ts
import { fileV2Api } from '@/api/fileV2'

const { data } = useQuery({
  queryKey: ['vfs', currentVirtualPath],
  queryFn: () => fileV2Api.list({ path: currentVirtualPath, page: 1, page_size: 100 }),
})
```
