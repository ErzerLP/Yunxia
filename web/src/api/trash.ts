import { apiClient } from './client'
import type { TrashItem } from '@/types/api'

export const trashApi = {
  list: (params?: { source_id?: number; page?: number; page_size?: number }) =>
    apiClient.get<{ items: TrashItem[] }>('/trash', { params }),

  restore: (id: number) =>
    apiClient.post<{ id: number; restored: boolean; restored_path?: string; restored_virtual_path?: string }>(`/trash/${id}/restore`),

  delete: (id: number) =>
    apiClient.delete<{ id: number; deleted: boolean }>(`/trash/${id}`),

  clear: (sourceId?: number) =>
    apiClient.delete<{ source_id?: number; cleared: boolean; deleted_count: number }>('/trash', { params: sourceId ? { source_id: sourceId } : undefined }),
}
