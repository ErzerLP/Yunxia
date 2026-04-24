import { apiClient } from './client'
import type {
  StorageSource,
  SourceDetailResponse,
  CreateSourceRequest,
  UpdateSourceRequest,
  TestSourceResponse,
  PaginationParams,
} from '@/types/api'

export const sourceApi = {
  list: (params?: PaginationParams & { view?: 'navigation' | 'admin' }) =>
    apiClient.get<{ items: StorageSource[]; view: string }>('/sources', { params }),
  get: (id: number) => apiClient.get<SourceDetailResponse>(`/sources/${id}`),
  create: (data: CreateSourceRequest) => apiClient.post<{ source: StorageSource }>('/sources', data),
  update: (id: number, data: UpdateSourceRequest) => apiClient.put<{ source: StorageSource }>(`/sources/${id}`, data),
  delete: (id: number) => apiClient.delete<{ deleted: boolean; id: number }>(`/sources/${id}`),
  test: (data: CreateSourceRequest) => apiClient.post<TestSourceResponse>('/sources/test', data),
  testById: (id: number) => apiClient.post<TestSourceResponse>(`/sources/${id}/test`),
}
