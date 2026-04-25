import { apiClient } from './client'
import type { AuditLog, AuditLogListResponse, PaginationParams } from '@/types/api'

export interface ListAuditParams extends PaginationParams {
  actor_user_id?: number;
  actor_role_key?: string;
  resource_type?: string;
  action?: string;
  result?: string;
  source_id?: number;
  virtual_path?: string;
  request_id?: string;
  entrypoint?: string;
  started_at?: string;
  ended_at?: string;
}

export const auditApi = {
  list: (params?: ListAuditParams) =>
    apiClient.get<AuditLogListResponse>('/audit/logs', { params }),

  get: (id: number) =>
    apiClient.get<{ log: AuditLog }>(`/audit/logs/${id}`),
}
