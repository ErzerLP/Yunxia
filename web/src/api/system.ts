import { apiClient } from './client'
import type { SystemConfigPublic, SystemVersion, HealthStatus } from '@/types/api'

export const systemApi = {
  getConfig: () => apiClient.get<SystemConfigPublic>('/system/config'),
  getVersion: () => apiClient.get<SystemVersion>('/system/version'),
  health: () => apiClient.get<HealthStatus>('/health'),
}
