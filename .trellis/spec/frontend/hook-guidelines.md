# Hook Guidelines

> How hooks are used in this project.

## Overview

Custom hooks live in `web/src/hooks/` and currently cover auth/capability guard behavior. Server data fetching is usually done directly in components/pages with TanStack Query; extract shared query hooks only when repeated.

Existing hooks:

- `useAuthGuard(requireAuth = true)` redirects based on auth state.
- `useSetupGuard()` currently does not redirect on its own because app initialization
  in `App.tsx` handles setup checks.
- `useHasCapability`, `useHasAnyCapability`, `useHasAllCapabilities` read from auth store.
- `useCapabilityGuard(cap, options)` enforces route-level capability and can show a toast.

## Custom Hook Patterns

- Hook names start with `use`.
- Hooks compose stores/router/query APIs; keep JSX out of hooks.
- Return plain state and callbacks: `{ allowed, isLoading }`.
- Include complete `useEffect` dependency arrays.
- Use options objects when optional behavior grows beyond one argument.

Example:

```ts
export function useCapabilityGuard(cap: string, options: UseCapabilityGuardOptions = {}) {
  const { isLoading, isAuthenticated, hasCapability } = useAuthStore()
  const navigate = useNavigate()
  const { addToast } = useUIStore()
  ...
  return { allowed, isLoading }
}
```

## Data Fetching

Use TanStack Query for server state.

Project defaults in `src/main.tsx`:

```ts
queries: {
  staleTime: 5 * 60 * 1000,
  retry: 1,
  refetchOnWindowFocus: false,
}
```

For file/VFS listing pages, override with `refetchOnMount: 'always'` so navigation shows fresh file state despite global stale time.

Query key conventions:

- VFS listing: `['vfs', currentVirtualPath]`
- Legacy files: `['files', sourceId, currentPath]` style
- Search: include keyword/current path or keep `enabled: false` with explicit `refetch`
- Management lists: stable feature keys such as `['shares']`, `['system-config']`, `['source-detail', id]`

Mutation success must invalidate relevant queries with `useQueryClient()`.

## Guards and Capabilities

Route-level guard:

```tsx
{ path: 'settings', element: <CapabilityRoute cap="system.config.read"><SettingsPage /></CapabilityRoute> }
```

Component-level guard:

```tsx
<CapabilityGuard cap="source.read" fallback={null}>...</CapabilityGuard>
```

Navigation filtering uses `useAuthStore().hasCapability` in `Sidebar`. Keep route guard and nav filtering aligned.

## Common Mistakes

- Redirecting while auth is still loading; existing guards return early if `isLoading`.
- Putting mutation side effects into `queryFn`; use event handlers or mutation hooks.
- Forgetting to invalidate both `['files']` and `['vfs']` after offline task completion/import.
- Creating a hook for one-off local UI state that is simpler as `useState`.

## Wrong vs Correct

### Wrong

```ts
useEffect(() => {
  if (!hasCapability(cap)) navigate('/files')
}, [])
```

### Correct

```ts
useEffect(() => {
  if (isLoading || !isAuthenticated) return
  if (!hasCapability(cap)) {
    if (showToast) addToast(toastMessage, 'error')
    navigate(redirectTo, { replace: true })
  }
}, [isLoading, isAuthenticated, cap, hasCapability, navigate, redirectTo, showToast, toastMessage, addToast])
```
