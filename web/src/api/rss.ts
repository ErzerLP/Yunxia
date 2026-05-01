import { apiClient } from './client'
import type {
  RSSItemStatus,
  RSSItemView,
  RSSQBitHealthResponse,
  RSSRefreshAllResponse,
  RSSRefreshResponse,
  RSSSourceUpsertRequest,
  RSSSourceView,
  RSSSubscriptionPreviewResponse,
  RSSSubscriptionUpsertRequest,
  RSSSubscriptionView,
} from '@/types/api'

export interface ListRSSSubscriptionsParams {
  source_id?: number
}

export interface ListRSSItemsParams {
  source_id?: number
  subscription_id?: number
  status?: RSSItemStatus
}

export interface RSSDeleteResponse {
  id: number
  deleted: boolean
}

function compactParams<T extends object>(params?: T) {
  if (!params) return undefined
  return Object.fromEntries(
    Object.entries(params).filter(([, value]) => value !== undefined && value !== null && value !== '')
  )
}

export const rssApi = {
  listSources: () =>
    apiClient.get<{ items: RSSSourceView[] }>('/rss/sources'),

  createSource: (data: RSSSourceUpsertRequest) =>
    apiClient.post<{ source: RSSSourceView }>('/rss/sources', data),

  getSource: (id: number) =>
    apiClient.get<RSSSourceView>(`/rss/sources/${id}`),

  updateSource: (id: number, data: RSSSourceUpsertRequest) =>
    apiClient.patch<{ source: RSSSourceView }>(`/rss/sources/${id}`, data),

  deleteSource: (id: number) =>
    apiClient.delete<RSSDeleteResponse>(`/rss/sources/${id}`),

  refreshSource: (id: number) =>
    apiClient.post<RSSRefreshResponse>(`/rss/sources/${id}/refresh`),

  refreshAllSources: () =>
    apiClient.post<RSSRefreshAllResponse>('/rss/sources/refresh-all'),

  listSubscriptions: (params?: ListRSSSubscriptionsParams) =>
    apiClient.get<{ items: RSSSubscriptionView[] }>('/rss/subscriptions', {
      params: compactParams(params),
    }),

  createSubscription: (data: RSSSubscriptionUpsertRequest) =>
    apiClient.post<{ subscription: RSSSubscriptionView }>('/rss/subscriptions', data),

  getSubscription: (id: number) =>
    apiClient.get<RSSSubscriptionView>(`/rss/subscriptions/${id}`),

  updateSubscription: (id: number, data: RSSSubscriptionUpsertRequest) =>
    apiClient.patch<{ subscription: RSSSubscriptionView }>(`/rss/subscriptions/${id}`, data),

  deleteSubscription: (id: number) =>
    apiClient.delete<RSSDeleteResponse>(`/rss/subscriptions/${id}`),

  runSubscription: (id: number) =>
    apiClient.post<RSSRefreshResponse>(`/rss/subscriptions/${id}/run`),

  previewSubscription: (id: number) =>
    apiClient.post<RSSSubscriptionPreviewResponse>(`/rss/subscriptions/${id}/preview`),

  listItems: (params?: ListRSSItemsParams) =>
    apiClient.get<{ items: RSSItemView[] }>('/rss/items', {
      params: compactParams(params),
    }),

  downloadItem: (id: number, subscriptionId?: number) =>
    apiClient.post<{ item: RSSItemView }>(
      `/rss/items/${id}/download`,
      subscriptionId ? { subscription_id: subscriptionId } : {}
    ),

  reprocessItem: (id: number) =>
    apiClient.post<{ item: RSSItemView }>(`/rss/items/${id}/reprocess`),

  retryItem: (id: number, subscriptionId?: number) =>
    apiClient.post<{ item: RSSItemView }>(
      `/rss/items/${id}/retry`,
      subscriptionId ? { subscription_id: subscriptionId } : {}
    ),

  qbitHealth: () =>
    apiClient.get<RSSQBitHealthResponse>('/rss/qbittorrent/health'),
}
