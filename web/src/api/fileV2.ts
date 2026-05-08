import { v2Client } from './client'
import type {
  AccessUrlResponse,
  PaginationParams,
  VFSAccessUrlRequest,
  VFSItem,
  VFSListResult,
  VFSRefreshResponse,
  VFSTag,
} from '@/types/api'

export interface ListVFSParams extends PaginationParams {
  path?: string;
}

export interface SearchVFSParams extends PaginationParams {
  keyword: string;
  path?: string;
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

export interface VFSRefreshRequest {
  path: string;
  mode?: 'sync';
}

export interface VFSTagRequest {
  path: string;
  tag_id: number;
}

export const fileV2Api = {
  list: (params?: ListVFSParams) =>
    v2Client.get<VFSListResult>('/fs/list', { params }),

  search: (params: SearchVFSParams) =>
    v2Client.get<VFSListResult>('/fs/search', { params }),

  mkdir: (data: VFSMkdirRequest) =>
    v2Client.post<{ created: VFSItem }>('/fs/mkdir', data),

  rename: (data: VFSRenameRequest) =>
    v2Client.post<{ old_path: string; new_path: string; file: VFSItem }>('/fs/rename', data),

  move: (data: VFSMoveRequest) =>
    v2Client.post<{ old_path: string; new_path: string; moved: boolean }>('/fs/move', data),

  copy: (data: VFSCopyRequest) =>
    v2Client.post<{ source_path: string; new_path: string; copied: boolean }>('/fs/copy', data),

  delete: (data: VFSDeleteRequest) =>
    v2Client.delete<{ deleted: boolean; delete_mode: string; path: string; deleted_at: string }>('/fs', { data }),

  refresh: (data: VFSRefreshRequest) =>
    v2Client.post<VFSRefreshResponse>('/fs/refresh', data),

  accessUrl: (data: VFSAccessUrlRequest) =>
    v2Client.post<AccessUrlResponse>('/fs/access-url', data),

  getTags: (path: string) =>
    v2Client.get<{ path: string; tags: VFSTag[] }>('/fs/tags', { params: { path } }),

  attachTag: (data: VFSTagRequest) =>
    v2Client.post<{ path: string; tags: VFSTag[] }>('/fs/tags/attach', data),

  detachTag: (data: VFSTagRequest) =>
    v2Client.post<{ path: string; tags: VFSTag[] }>('/fs/tags/detach', data),

  download: (path: string) => {
    const encoded = encodeURIComponent(path)
    return `/api/v2/fs/download?path=${encoded}`
  },
}
