import axios from 'axios'

// Public share endpoints are NOT under /api/v1, use raw axios
// Use full backend URL to avoid Vite proxy intercepting /s/ page navigation
const BACKEND_URL = (import.meta.env.VITE_BACKEND_URL || 'http://localhost:8080').replace(/\/+$/, '')
const PUBLIC_SHARE_BASE_URL = (
  import.meta.env.VITE_PUBLIC_SHARE_BASE_URL ||
  (import.meta.env.DEV ? '/__public_share' : `${BACKEND_URL}/s`)
).replace(/\/+$/, '')
const publicClient = axios.create({
  baseURL: PUBLIC_SHARE_BASE_URL,
  timeout: 30000,
  headers: { 'Content-Type': 'application/json' },
})

export interface PublicShareEntry {
  name: string
  path: string
  parent_path: string
  is_dir: boolean
  preview_type: string
  size: number
  mime_type: string
  extension: string
  modified_at: string
  created_at: string
  can_preview: boolean
  can_download: boolean
  thumbnail_url: string | null
}

export interface PublicShareInfo {
  id: number
  source_id: number
  path: string
  name: string
  is_dir: boolean
  link: string
  has_password: boolean
  expires_at: string | null
  created_at: string
}

export interface PublicShareOpenResponse {
  share: PublicShareInfo
  current_path: string
  current_dir: {
    name: string
    path: string
    parent_path: string
    is_root: boolean
  }
  breadcrumbs: { name: string; path: string }[]
  pagination: {
    page: number
    page_size: number
    total: number
    total_pages: number
  }
  items: PublicShareEntry[]
}

export interface PublicShareOpenUrlOptions {
  password?: string
  path?: string
  disposition?: 'inline' | 'attachment'
}

function extractPublicShareError(err: unknown): Error {
  if (axios.isAxiosError(err)) {
    const data = err.response?.data as { message?: string; code?: string } | undefined
    return new Error(data?.message || data?.code || err.message)
  }
  return err instanceof Error ? err : new Error('分享访问失败')
}

export const sharePublicApi = {
  getOpenUrl: (token: string, options: PublicShareOpenUrlOptions = {}) => {
    const params = new URLSearchParams()
    if (options.password) params.set('password', options.password)
    if (options.path) params.set('path', options.path)
    if (options.disposition) params.set('disposition', options.disposition)
    const query = params.toString()
    return `${PUBLIC_SHARE_BASE_URL}/${encodeURIComponent(token)}${query ? `?${query}` : ''}`
  },

  open: async (token: string, password?: string, path?: string) => {
    let r
    try {
      r = await publicClient.get<unknown>(`/${token}`, {
        params: { password, path: path || '/' },
      })
    } catch (err: unknown) {
      throw extractPublicShareError(err)
    }

    const responseData = r.data as Record<string, unknown>
    if (!responseData || typeof responseData !== 'object') {
      throw new Error('SHARE_FILE_REDIRECT')
    }
    // Backend envelope: { success, code, message, data: PublicShareOpenResponse }
    const innerData = responseData.data as PublicShareOpenResponse | undefined
    if (!innerData) {
      throw new Error('SHARE_FILE_REDIRECT')
    }
    return innerData
  },
}
