import { apiClient } from './client'
import type {
  FileItem,
  FileListResult,
  ListFilesParams,
  SearchFilesParams,
  MkdirRequest,
  RenameRequest,
  MoveRequest,
  CopyRequest,
  DeleteRequest,
  AccessUrlRequest,
  AccessUrlResponse,
} from '@/types/api'

export const fileApi = {
  list: (params: ListFilesParams) =>
    apiClient.get<FileListResult>('/files', { params }),
  search: (params: SearchFilesParams) =>
    apiClient.get<{ items: FileItem[]; total: number }>('/files/search', { params }),
  mkdir: (data: MkdirRequest) =>
    apiClient.post<{ created: FileItem }>('/files/mkdir', data),
  rename: (data: RenameRequest) =>
    apiClient.post<{ old_path: string; new_path: string; file: FileItem }>('/files/rename', data),
  move: (data: MoveRequest) =>
    apiClient.post<{ old_path: string; new_path: string; moved: boolean }>('/files/move', data),
  copy: (data: CopyRequest) =>
    apiClient.post<{ source_path: string; new_path: string; copied: boolean }>('/files/copy', data),
  delete: (data: DeleteRequest) =>
    apiClient.delete<void>('/files', { data }),
  getAccessUrl: (data: AccessUrlRequest) =>
    apiClient.post<AccessUrlResponse>('/files/access-url', data),
}
