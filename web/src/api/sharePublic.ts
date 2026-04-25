import axios from 'axios'

// Public share endpoints are NOT under /api/v1, use raw axios
const publicClient = axios.create({
  baseURL: '',
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

export const sharePublicApi = {
  open: async (token: string, password?: string, path?: string) => {
    const r = await publicClient.get<unknown>(`/s/${token}`, {
      params: { password, path: path || '/' },
    })
    const responseData = r.data as Record<string, unknown>
    if (!responseData || typeof responseData !== 'object') {
      throw new Error('Invalid response format')
    }
    // Backend envelope: { success, code, message, data: PublicShareOpenResponse }
    const innerData = responseData.data as PublicShareOpenResponse | undefined
    if (!innerData) {
      throw new Error('Response missing data field')
    }
    return innerData
  },
}
