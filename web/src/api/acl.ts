import { apiClient } from './client'
import type { AclRule, CreateAclRuleRequest, PaginationParams } from '@/types/api'

export interface ListAclParams extends PaginationParams {
  source_id: number;
  path?: string;
  subject_type?: string;
  subject_id?: number;
}

export const aclApi = {
  list: (params: ListAclParams) =>
    apiClient.get<{ items: AclRule[]; total: number }>('/acl/rules', { params }),

  create: (data: CreateAclRuleRequest) =>
    apiClient.post<{ rule: AclRule }>('/acl/rules', data),

  update: (id: number, data: Partial<CreateAclRuleRequest>) =>
    apiClient.put<{ rule: AclRule }>(`/acl/rules/${id}`, data),

  delete: (id: number) =>
    apiClient.delete<{ id: number; deleted: boolean }>(`/acl/rules/${id}`),
}
