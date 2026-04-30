# Component Guidelines

> How components are built in this project.

## Overview

Yunxia uses function components, TypeScript props interfaces, Tailwind utility classes, semantic UI tokens, and the `cn` helper from `@/utils` for conditional classes. Existing components use direct function declarations rather than `React.FC`.

```tsx
interface VFSMkdirModalProps {
  isOpen: boolean
  onClose: () => void
  parentPath: string
  onSuccess?: () => void
}

export function VFSMkdirModal({ isOpen, onClose, parentPath, onSuccess }: VFSMkdirModalProps) {
  ...
}
```

## Component Structure

Recommended order:

1. Imports: React hooks, third-party icons/libs, project API/stores/utils, types.
2. Local constants/maps.
3. Props interfaces.
4. Small local subcomponents if tightly coupled.
5. Exported component.
6. Early loading/empty/null states before main return when clear.

Keep route orchestration in pages and reusable domain UI in components.

## Props Conventions

- Use explicit `interface <ComponentName>Props` for components with several props.
- Optional callbacks use `onSuccess?: () => void` and optional chaining.
- Children props use `React.ReactNode` where needed.
- Prefer narrow string unions for finite behavior: `mode: 'move' | 'copy'`, `type: 'success' | 'error' | 'warning' | 'info'`.

## Styling Patterns

- Default to semantic Tailwind tokens: `bg-background`, `text-foreground`, `border-border`, `bg-card`, `text-muted-foreground`, `text-destructive`, `bg-primary`.
- Use `cn(...)` for conditional classes:

```tsx
className={cn(
  'p-2 rounded-md transition-colors',
  canGoUp ? 'hover:bg-accent text-muted-foreground' : 'text-muted-foreground/30 cursor-not-allowed'
)}
```

- Use `lucide-react` icons with fixed sizing (`w-4 h-4`, `w-5 h-5`) and `shrink-0` where needed.
- Theme variables live in `src/index.css` and Tailwind config. Do not build separate component-level design systems.
- Existing stat/status accents may use a small fixed Tailwind palette when
  contained behind props, e.g. `SettingsPage` `StatCard` receives
  `bg-blue-500` / `bg-emerald-500`. Keep arbitrary colors localized this way;
  do not scatter one-off color utilities through reusable components.

## Interaction Patterns

- Forms should keep local `isSubmitting` and visible `error` state when mutations can fail.
- Use `useUIStore().addToast` for user-visible success/error feedback.
- After successful mutations, invalidate/refetch the relevant TanStack Query key and/or call `onSuccess`.
- File/VFS lists should render current query data first to avoid transient empty states:

```tsx
const displayedVfsItems = data?.items ?? vfsItems
```

## Accessibility

When adding forms:

- Pair labels with inputs using `htmlFor` and matching `id`.
- Provide `name` for browser/autofill/a11y support.
- Use correct `autoComplete` values (`current-password`, `new-password`, `username`).
- Icon-only buttons need `title` or accessible text.
- Disabled buttons should use `disabled` and visible disabled styles.

## Common Mistakes

- Empty `catch {}` on user actions; show toast and/or inline error.
- Direct VFS downloads with `window.open(fileV2Api.download(path))`; VFS must first call `fileV2Api.accessUrl`.
- Reintroducing a separate “VFS” sidebar entry; `/files` is canonical.
- Rendering only store data when query data exists; use query data first.

## Wrong vs Correct

### Wrong

```tsx
<button onClick={async () => { await apiCall() }}>Save</button>
```

### Correct

```tsx
try {
  await fileV2Api.mkdir({ parent_path: parentPath, name: trimmed })
  addToast('文件夹创建成功', 'success')
  onSuccess?.()
  onClose()
} catch (err: unknown) {
  const msg = err instanceof Error ? err.message : '创建失败'
  setError(msg)
  addToast(msg, 'error')
}
```
