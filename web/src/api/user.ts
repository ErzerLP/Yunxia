import { apiClient } from './client'
import type { User, CreateUserRequest, UpdateUserRequest, PaginationParams } from '@/types/api'

export const userApi = {
  list: (params?: PaginationParams) =>
    apiClient.get<{ items: User[]; total: number }>('/users', { params }),
  create: (data: CreateUserRequest) =>
    apiClient.post<{ user: User }>('/users', data),
  update: (id: number, data: UpdateUserRequest) =>
    apiClient.put<{ user: User }>(`/users/${id}`, data),
  delete: (id: number) =>
    apiClient.delete<{ deleted: boolean; id: number }>(`/users/${id}`),
  resetPassword: (id: number, password: string) =>
    apiClient.post<{}>(`/users/${id}/reset-password`, { password }),
  revokeTokens: (id: number) =>
    apiClient.post<{}>(`/users/${id}/revoke-tokens`, {}),
}
