import { apiClient } from './client'
import type {
  RSSItemStatus,
  RSSItemBatchActionResponse,
  RSSItemView,
  RSSExportResponse,
  RSSImportRequest,
  RSSImportResponse,
  RSSQBitHealthResponse,
  RSSRefreshAllResponse,
  RSSRefreshResponse,
  RSSSourceUpsertRequest,
  RSSSourceView,
  RSSSubscriptionBatchStateRequest,
  RSSSubscriptionBatchStateResponse,
  RSSSubscriptionCloneRequest,
  RSSSubscriptionPreviewRequest,
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

  exportConfig: () =>
    apiClient.get<RSSExportResponse>('/rss/export'),

  importConfig: (data: RSSImportRequest) =>
    apiClient.post<RSSImportResponse>('/rss/import', data),

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

  cloneSubscription: (id: number, data?: RSSSubscriptionCloneRequest) =>
    apiClient.post<{ subscription: RSSSubscriptionView }>(`/rss/subscriptions/${id}/clone`, data ?? {}),

  batchSubscriptionState: (data: RSSSubscriptionBatchStateRequest) =>
    apiClient.post<RSSSubscriptionBatchStateResponse>('/rss/subscriptions/batch-state', data),

  runSubscription: (id: number) =>
    apiClient.post<RSSRefreshResponse>(`/rss/subscriptions/${id}/run`),

  previewSubscription: (id: number) =>
    apiClient.post<RSSSubscriptionPreviewResponse>(`/rss/subscriptions/${id}/preview`),

  previewSubscriptionDraft: (data: RSSSubscriptionPreviewRequest) =>
    apiClient.post<RSSSubscriptionPreviewResponse>('/rss/subscriptions/preview', data),

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

  batchIgnoreItems: (itemIds: number[]) =>
    apiClient.post<RSSItemBatchActionResponse>('/rss/items/batch-ignore', { item_ids: itemIds }),

  batchRetryItems: (itemIds: number[], subscriptionId?: number) =>
    apiClient.post<RSSItemBatchActionResponse>(
      '/rss/items/batch-retry',
      subscriptionId ? { item_ids: itemIds, subscription_id: subscriptionId } : { item_ids: itemIds }
    ),

  qbitHealth: () =>
    apiClient.get<RSSQBitHealthResponse>('/rss/qbittorrent/health'),
}
