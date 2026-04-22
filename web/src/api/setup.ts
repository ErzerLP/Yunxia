import { apiClient } from './client'
import type { SetupStatus, SetupInitRequest, SetupInitResponse } from '@/types/api'

export const setupApi = {
  getStatus: () => apiClient.get<SetupStatus>('/setup/status'),
  initialize: (data: SetupInitRequest) => apiClient.post<SetupInitResponse>('/setup/init', data),
}
