import { apiClient } from './client'
import type { VFSTag } from '@/types/api'

export interface UpsertTagRequest {
  name: string
  color?: string
}

export const tagApi = {
  list: () =>
    apiClient.get<{ items: VFSTag[] }>('/tags'),

  create: (data: UpsertTagRequest) =>
    apiClient.post<{ tag: VFSTag }>('/tags', data),

  update: (id: number, data: UpsertTagRequest) =>
    apiClient.patch<{ tag: VFSTag }>(`/tags/${id}`, data),

  delete: (id: number) =>
    apiClient.delete<{ deleted: boolean; id: number }>(`/tags/${id}`),
}
