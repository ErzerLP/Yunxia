import axios from 'axios'
import type { FileItem } from '@/types/api'

// Public share endpoints are NOT under /api/v1, use raw axios
const publicClient = axios.create({
  baseURL: '',
  timeout: 30000,
  headers: { 'Content-Type': 'application/json' },
})

export interface PublicShareInfo {
  name: string
  is_dir: boolean
  has_password: boolean
  expires_at: string | null
}

export interface PublicShareOpenResponse {
  name: string
  is_dir: boolean
  has_password: boolean
  expires_at: string | null
  items?: FileItem[]
  current_path?: string
}

export const sharePublicApi = {
  open: (token: string, password?: string, path?: string) =>
    publicClient.get<{ data: PublicShareOpenResponse }>(`/s/${token}`, {
      params: { password, path: path || '/' },
    }).then(r => r.data.data),

  accessUrl: (token: string, path: string, purpose: 'preview' | 'download' = 'download') =>
    publicClient.get<{ data: { url: string; method: string; expires_at: string } }>(`/s/${token}`, {
      params: { path, disposition: purpose === 'download' ? 'attachment' : 'inline' },
    }).then(r => r.data.data),
}
