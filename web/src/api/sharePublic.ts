import { apiClient } from './client'
import type { FileItem } from '@/types/api'

export interface PublicShareInfo {
  name: string
  is_dir: boolean
  has_password: boolean
  expires_at: string | null
}

export interface PublicShareVerifyRequest {
  token: string
  password?: string
}

export interface PublicShareVerifyResponse {
  share: PublicShareInfo
  access_token?: string
}

export interface PublicShareListResponse {
  items: FileItem[]
  current_path: string
}

export const sharePublicApi = {
  verify: (token: string, password?: string) =>
    apiClient.post<PublicShareVerifyResponse>('/shares/public/verify', { token, password }),

  list: (token: string, path?: string, accessToken?: string) =>
    apiClient.get<PublicShareListResponse>(`/shares/public/${token}/list`, {
      params: { path: path || '/' },
      headers: accessToken ? { 'X-Share-Access-Token': accessToken } : undefined,
    }),

  accessUrl: (token: string, path: string, accessToken?: string, purpose: 'preview' | 'download' = 'download') =>
    apiClient.get<{ url: string; method: string; expires_at: string }>(`/shares/public/${token}/access`, {
      params: { path, purpose },
      headers: accessToken ? { 'X-Share-Access-Token': accessToken } : undefined,
    }),
}
