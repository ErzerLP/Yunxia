const DEFAULT_WEBDAV_PREFIX = '/dav'

function getDefaultOrigin() {
  if (typeof window === 'undefined') return ''
  return window.location.origin
}

export function normalizeWebDAVPrefix(prefix?: string | null) {
  const raw = prefix?.trim() || DEFAULT_WEBDAV_PREFIX
  const withLeadingSlash = raw.startsWith('/') ? raw : `/${raw}`
  const withoutTrailingSlash = withLeadingSlash.replace(/\/+$/g, '')
  return withoutTrailingSlash || '/'
}

export function buildWebDAVBaseUrl(prefix?: string | null, origin = getDefaultOrigin()) {
  const normalizedPrefix = normalizeWebDAVPrefix(prefix)
  const normalizedOrigin = origin.replace(/\/+$/g, '')
  const path = normalizedPrefix === '/' ? '/' : `${normalizedPrefix}/`

  return `${normalizedOrigin}${path}`
}

export function buildSourceWebDAVUrl(prefix: string | null | undefined, slug: string, origin = getDefaultOrigin()) {
  const normalizedSlug = slug.trim()
  if (!normalizedSlug) return ''

  return `${buildWebDAVBaseUrl(prefix, origin)}${encodeURIComponent(normalizedSlug)}/`
}
