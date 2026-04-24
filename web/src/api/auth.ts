import { apiClient } from './client'
import type { LoginRequest, LoginResponse, RefreshTokenRequest, RefreshTokenResponse, LogoutRequest, CurrentUserResponse } from '@/types/api'

export const authApi = {
  login: (data: LoginRequest) => apiClient.post<LoginResponse>('/auth/login', data),
  refresh: (data: RefreshTokenRequest) => apiClient.post<RefreshTokenResponse>('/auth/refresh', data),
  logout: (data: LogoutRequest) => apiClient.post<void>('/auth/logout', data),
  me: () => apiClient.get<CurrentUserResponse>('/auth/me'),
}
