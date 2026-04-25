import { apiClient } from './client'
import type { VFSItem, VFSListResult, VFSAccessUrlRequest, AccessUrlResponse, PaginationParams } from '@/types/api'

export interface ListVFSParams extends PaginationParams {
  path?: string;
}

export interface SearchVFSParams extends PaginationParams {
  keyword: string;
  path_prefix?: string;
}

export interface VFSMkdirRequest {
  parent_path: string;
  name: string;
}

export interface VFSRenameRequest {
  path: string;
  new_name: string;
}

export interface VFSMoveRequest {
  path: string;
  target_path: string;
}

export interface VFSCopyRequest {
  path: string;
  target_path: string;
}

export interface VFSDeleteRequest {
  path: string;
  delete_mode?: 'trash' | 'permanent';
}

export const fileV2Api = {
  list: (params?: ListVFSParams) =>
    apiClient.get<VFSListResult>('/api/v2/fs/list', { params }),

  search: (params: SearchVFSParams) =>
    apiClient.get<VFSListResult>('/api/v2/fs/search', { params }),

  mkdir: (data: VFSMkdirRequest) =>
    apiClient.post<{ item: VFSItem }>('/api/v2/fs/mkdir', data),

  rename: (data: VFSRenameRequest) =>
    apiClient.post<{ item: VFSItem }>('/api/v2/fs/rename', data),

  move: (data: VFSMoveRequest) =>
    apiClient.post<{ moved: number }>('/api/v2/fs/move', data),

  copy: (data: VFSCopyRequest) =>
    apiClient.post<{ copied: number; item?: VFSItem }>('/api/v2/fs/copy', data),

  delete: (data: VFSDeleteRequest) =>
    apiClient.delete<{ deleted: number }>('/api/v2/fs', { data }),

  accessUrl: (data: VFSAccessUrlRequest) =>
    apiClient.post<AccessUrlResponse>('/api/v2/fs/access-url', data),

  download: (path: string) => {
    const encoded = encodeURIComponent(path)
    return `/api/v2/fs/download?path=${encoded}`
  },
}
