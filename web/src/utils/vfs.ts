import type { CreateShareRequest, StorageSource, VFSItem } from '@/types/api'

export function normalizeVfsPath(value: string | null | undefined): string {
  let path = (value || '/').trim()
  if (!path.startsWith('/')) path = `/${path}`
  path = path.replace(/\/+/g, '/')
  if (path.length > 1) path = path.replace(/\/+$/g, '')
  return path || '/'
}

export function resolveInnerPathFromMount(virtualPath: string, mountPath: string): string | null {
  const normalizedVirtualPath = normalizeVfsPath(virtualPath)
  const normalizedMountPath = normalizeVfsPath(mountPath)

  if (normalizedMountPath === '/') {
    return normalizedVirtualPath
  }
  if (normalizedVirtualPath === normalizedMountPath) {
    return '/'
  }
  if (!normalizedVirtualPath.startsWith(`${normalizedMountPath}/`)) {
    return null
  }

  return normalizeVfsPath(normalizedVirtualPath.slice(normalizedMountPath.length))
}

export function buildVfsShareRequest(
  item: VFSItem,
  sources: StorageSource[],
): CreateShareRequest | null {
  const source = item.source_id == null
    ? sources
        .slice()
        .sort((a, b) => normalizeVfsPath(b.mount_path).length - normalizeVfsPath(a.mount_path).length)
        .find((candidate) => resolveInnerPathFromMount(item.path, candidate.mount_path) !== null)
    : sources.find((candidate) => candidate.id === item.source_id)

  const innerPath = source ? resolveInnerPathFromMount(item.path, source.mount_path) : null
  const legacyTarget = source && innerPath
    ? {
        source_id: source.id,
        path: innerPath,
      }
    : null

  if (item.id) {
    return {
      vfs_node_id: item.id,
      ...(legacyTarget ?? {}),
    }
  }

  return legacyTarget
}

export function getVfsParentPath(path: string): string {
  const normalizedPath = normalizeVfsPath(path)
  if (normalizedPath === '/') return '/'
  const parts = normalizedPath.split('/').filter(Boolean)
  parts.pop()
  return parts.length === 0 ? '/' : `/${parts.join('/')}`
}

export function getVfsBasename(path: string): string {
  const normalizedPath = normalizeVfsPath(path)
  if (normalizedPath === '/') return '/'
  const parts = normalizedPath.split('/').filter(Boolean)
  return parts.at(-1) ?? '/'
}

export function toAbsoluteShareLink(link: string): string {
  if (/^https?:\/\//i.test(link)) return link
  return `${window.location.origin}${link.startsWith('/') ? link : `/${link}`}`
}

export function toFrontendShareLink(link: string): string {
  try {
    const parsed = /^https?:\/\//i.test(link)
      ? new URL(link)
      : new URL(link.startsWith('/') ? link : `/${link}`, window.location.origin)
    return `${window.location.origin}${parsed.pathname}${parsed.search}${parsed.hash}`
  } catch {
    return `${window.location.origin}${link.startsWith('/') ? link : `/${link}`}`
  }
}
