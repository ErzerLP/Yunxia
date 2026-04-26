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
  if (item.source_id == null) return null

  const source = sources.find((candidate) => candidate.id === item.source_id)
  if (!source) return null

  const innerPath = resolveInnerPathFromMount(item.path, source.mount_path)
  if (!innerPath) return null

  return {
    source_id: item.source_id,
    path: innerPath,
  }
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
