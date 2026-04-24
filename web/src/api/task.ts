import { apiClient } from './client'
import type { DownloadTask, CreateTaskRequest, PaginationParams } from '@/types/api'

export interface TaskActionResponse {
  id: number
  status: string
}

export interface CancelTaskResponse {
  id: number
  canceled: boolean
  delete_file: boolean
}

export const taskApi = {
  list: (params?: PaginationParams) =>
    apiClient.get<{ items: DownloadTask[] }>('/tasks', { params }),
  create: (data: CreateTaskRequest) =>
    apiClient.post<{ task: DownloadTask }>('/tasks', data),
  get: (id: number) =>
    apiClient.get<DownloadTask>(`/tasks/${id}`),
  pause: (id: number) =>
    apiClient.post<TaskActionResponse>(`/tasks/${id}/pause`),
  resume: (id: number) =>
    apiClient.post<TaskActionResponse>(`/tasks/${id}/resume`),
  cancel: (id: number, deleteFile = false) =>
    apiClient.delete<CancelTaskResponse>(`/tasks/${id}`, {
      params: { delete_file: deleteFile },
    }),
}
