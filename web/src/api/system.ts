import { apiClient } from './client'
import type { SystemConfigPublic, SystemVersion, HealthStatus, SystemStats } from '@/types/api'

export const systemApi = {
  getConfig: () => apiClient.get<SystemConfigPublic>('/system/config'),
  getVersion: () => apiClient.get<SystemVersion>('/system/version'),
  getStats: () => apiClient.get<SystemStats>('/system/stats'),
  updateConfig: (data: SystemConfigPublic) =>
    apiClient.put<SystemConfigPublic>('/system/config', data),
  health: () => apiClient.get<HealthStatus>('/health'),
}
