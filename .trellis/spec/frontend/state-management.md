# State Management

> How state is managed in this project.

## Overview

Yunxia separates state into:

1. Server state: TanStack Query.
2. Client/app state: Zustand stores in `src/stores/`.
3. Local component state: `useState`/`useRef` for forms, modals, context menus, transient UI.

Do not put server lists into Zustand as the primary source of truth. File pages may mirror query data into stores for coordination, but render current query data first where possible.

## Zustand Stores

Current stores:

- `authStore.ts`: current user, capabilities, auth loading flag, persisted auth summary, token helpers.
- `fileStore.ts`: V1/V2 file mode, current source/path/virtual path, selected files, view/sort, current permissions.
- `uiStore.ts`: sidebar, preview drawer, theme, global upload modal, loading, toasts.

### Auth store contract

- `setTokens` writes `access_token` and `refresh_token` to localStorage.
- `logout` removes tokens and clears user/capabilities.
- `hasCapability(cap)` checks current capabilities.
- Persisted state name is `auth-storage`; only user/capabilities/isAuthenticated are persisted.

### File store contract

- `mode: 'v1' | 'v2'` controls upload/preview behavior.
- `VFSFileManagerPage` must call `setMode('v2')` on entry.
- Legacy `FileManagerPage` must call `setMode('v1')` on entry.
- VFS paths use `currentVirtualPath`; legacy paths use `currentPath` + `currentSource`.
- Changing mode/source/path clears selected files and current permissions.

### UI store contract

- Preview drawer stores mode (`v1`/`v2`), path, optional source ID, filename, MIME type.
- Toasts are stored in `uiStore` and rendered by `ToastContainer` in `AppLayout`.
- Theme is stored in localStorage key `theme` and applied to `document.documentElement.classList`.

## Server State

Use typed API modules in `useQuery`:

```ts
const { data, isLoading } = useQuery({
  queryKey: ['vfs', currentVirtualPath],
  queryFn: () => fileV2Api.list({ path: currentVirtualPath, page: 1, page_size: 100 }),
  refetchOnMount: 'always',
})
```

On mutation success:

```ts
queryClient.invalidateQueries({ queryKey: ['vfs', currentVirtualPath] })
queryClient.invalidateQueries({ queryKey: ['shares'] })
```

## Local State

Use local state for input values, modal open flags, context menu coordinates, submit/loading/error status, and search text before execution. Example: `VFSMkdirModal` keeps `name`, `isSubmitting`, and `error` local.

## State Matrix

| State kind | Owner | Example | Do not |
|---|---|---|---|
| Auth identity/capabilities | `authStore` | `user`, `capabilities` | Duplicate in page state |
| API lists/details | TanStack Query | VFS list, audit logs | Treat Zustand mirror as canonical |
| File selection/navigation | `fileStore` | `selectedFiles`, `currentVirtualPath` | Encode selection only in query cache |
| Toasts/theme/sidebar | `uiStore` | `addToast`, `theme` | Create competing toast systems |
| Form draft | component local state | mkdir name, create-source form | Put every input into global store |

## Common Mistakes

- Forgetting to reset file mode between V1 and V2 pages.
- Mutating a `Set` in place in Zustand.
- Relying on global `staleTime` for file lists; file views need refresh/invalidation.
- Leaving tokens in localStorage after logout/refresh failure.

## Wrong vs Correct

### Wrong

```ts
state.selectedFiles.add(path)
return { selectedFiles: state.selectedFiles }
```

### Correct

```ts
set((state) => {
  const next = new Set(state.selectedFiles)
  next.has(path) ? next.delete(path) : next.add(path)
  return { selectedFiles: next }
})
```
