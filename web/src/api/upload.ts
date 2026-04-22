import { apiClient } from './client'
import type {
  ApiResponse,
  UploadInitRequest,
  UploadInitResponse,
  UploadChunkResponse,
  UploadFinishRequest,
  UploadFinishResponse,
  UploadSession,
} from '@/types/api'

export const uploadApi = {
  init: (data: UploadInitRequest) =>
    apiClient.post<UploadInitResponse>('/upload/init', data),

  uploadChunk: (uploadId: string, index: number, chunk: Blob) => {
    const instance = apiClient.getRawInstance()
    return instance.put<ApiResponse<UploadChunkResponse>>(
      `/upload/chunk?upload_id=${uploadId}&index=${index}`,
      chunk,
      {
        headers: { 'Content-Type': 'application/octet-stream' },
      }
    ).then(res => res.data.data)
  },

  finish: (data: UploadFinishRequest) =>
    apiClient.post<UploadFinishResponse>('/upload/finish', data),

  listSessions: (params?: { status?: string; source_id?: number }) =>
    apiClient.get<{ items: UploadSession[] }>('/upload/sessions', { params }),

  cancelSession: (uploadId: string) =>
    apiClient.delete<{ upload_id: string; canceled: boolean }>(`/upload/sessions/${uploadId}`),
}
