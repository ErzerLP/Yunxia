import { apiClient } from './client'
import type { Share, CreateShareRequest } from '@/types/api'

export const shareApi = {
  list: () =>
    apiClient.get<{ items: Share[] }>('/shares'),

  get: (id: number) =>
    apiClient.get<{ share: Share }>(`/shares/${id}`),

  create: (data: CreateShareRequest) =>
    apiClient.post<{ share: Share }>('/shares', data),

  update: (id: number, data: { expires_in?: number; password?: string | null }) =>
    apiClient.put<{ share: Share }>(`/shares/${id}`, data),

  delete: (id: number) =>
    apiClient.delete<{ id: number; deleted: boolean }>(`/shares/${id}`),
}
