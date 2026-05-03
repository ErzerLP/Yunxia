import { apiClient } from './client'
import type {
  NotificationChannelUpsertRequest,
  NotificationChannelView,
  NotificationEventStatus,
  NotificationEventType,
  NotificationEventView,
} from '@/types/api'

export interface ListNotificationEventsParams {
  status?: NotificationEventStatus
  event_type?: NotificationEventType
  limit?: number
}

export interface NotificationDeleteResponse {
  id: number
  deleted: boolean
}

function compactParams<T extends object>(params?: T) {
  if (!params) return undefined
  return Object.fromEntries(
    Object.entries(params).filter(([, value]) => value !== undefined && value !== null && value !== '')
  )
}

export const notificationApi = {
  listChannels: () =>
    apiClient.get<{ items: NotificationChannelView[] }>('/notifications/channels'),

  createChannel: (data: NotificationChannelUpsertRequest) =>
    apiClient.post<{ channel: NotificationChannelView }>('/notifications/channels', data),

  updateChannel: (id: number, data: NotificationChannelUpsertRequest) =>
    apiClient.put<{ channel: NotificationChannelView }>(`/notifications/channels/${id}`, data),

  deleteChannel: (id: number) =>
    apiClient.delete<NotificationDeleteResponse>(`/notifications/channels/${id}`),

  testChannel: (id: number) =>
    apiClient.post<{ ok: boolean }>(`/notifications/channels/${id}/test`),

  listEvents: (params?: ListNotificationEventsParams) =>
    apiClient.get<{ items: NotificationEventView[] }>('/notifications/events', {
      params: compactParams(params),
    }),

  retryEvent: (id: number) =>
    apiClient.post<{ event: NotificationEventView }>(`/notifications/events/${id}/retry`),
}
