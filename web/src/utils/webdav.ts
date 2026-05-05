const DEFAULT_WEBDAV_PREFIX = '/dav'

function getDefaultOrigin() {
  if (typeof window === 'undefined') return ''
  return window.location.origin
}

export function toSecureWebDAVOrigin(origin = getDefaultOrigin()) {
  const trimmed = origin.trim().replace(/\/+$/g, '')
  if (!trimmed) return ''

  try {
    const url = new URL(trimmed)
    if (url.protocol === 'http:') {
      url.protocol = 'https:'
    }
    return url.toString().replace(/\/+$/g, '')
  } catch {
    return trimmed.replace(/^http:\/\//i, 'https://')
  }
}

export function isWebDAVOriginPromotedToHttps(origin = getDefaultOrigin()) {
  return /^http:\/\//i.test(origin.trim()) && toSecureWebDAVOrigin(origin) !== origin.trim().replace(/\/+$/g, '')
}

export function normalizeWebDAVPrefix(prefix?: string | null) {
  const raw = prefix?.trim() || DEFAULT_WEBDAV_PREFIX
  const withLeadingSlash = raw.startsWith('/') ? raw : `/${raw}`
  const withoutTrailingSlash = withLeadingSlash.replace(/\/+$/g, '')
  return withoutTrailingSlash || '/'
}

export function buildWebDAVBaseUrl(prefix?: string | null, origin = getDefaultOrigin()) {
  const normalizedPrefix = normalizeWebDAVPrefix(prefix)
  const normalizedOrigin = toSecureWebDAVOrigin(origin)
  const path = normalizedPrefix === '/' ? '/' : `${normalizedPrefix}/`

  return `${normalizedOrigin}${path}`
}

export function buildSourceWebDAVUrl(prefix: string | null | undefined, slug: string, origin = getDefaultOrigin()) {
  const normalizedSlug = slug.trim()
  if (!normalizedSlug) return ''

  return `${buildWebDAVBaseUrl(prefix, origin)}${encodeURIComponent(normalizedSlug)}/`
}
