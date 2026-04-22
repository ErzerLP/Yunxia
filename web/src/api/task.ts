import { apiClient } from './client'
import type { DownloadTask, CreateTaskRequest, PaginationParams } from '@/types/api'

export const taskApi = {
  list: (params?: PaginationParams) =>
    apiClient.get<{ items: DownloadTask[]; total: number }>('/tasks', { params }),
  create: (data: CreateTaskRequest) =>
    apiClient.post<{ task: DownloadTask }>('/tasks', data),
  get: (id: number) =>
    apiClient.get<DownloadTask>(`/tasks/${id}`),
  pause: (id: number) =>
    apiClient.post<{ id: number; status: string }>(`/tasks/${id}/pause`),
  resume: (id: number) =>
    apiClient.post<{ id: number; status: string }>(`/tasks/${id}/resume`),
  cancel: (id: number, deleteFile = false) =>
    apiClient.delete<{ id: number; canceled: boolean; delete_file: boolean }>(`/tasks/${id}`, {
      params: { delete_file: deleteFile },
    }),
  retry: (id: number) =>
    apiClient.post<DownloadTask>(`/tasks/${id}/retry`),
  delete: (id: number) =>
    apiClient.delete<void>(`/tasks/${id}`),
}
