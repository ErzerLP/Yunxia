// API 请求/响应类型定义

export interface ApiResponse<T> {
  success: boolean;
  code: string;
  message: string;
  data: T;
  meta: {
    request_id: string;
    timestamp: string;
    pagination?: PaginationMeta;
  };
}

export interface ApiError {
  success: false;
  code: string;
  message: string;
  error: {
    details?: Record<string, unknown>;
  };
  meta: {
    request_id: string;
    timestamp: string;
  };
}

export interface PaginationMeta {
  page: number;
  page_size: number;
  total: number;
  total_pages: number;
}

export interface PaginationParams {
  page?: number;
  page_size?: number;
  sort_by?: string;
  sort_order?: 'asc' | 'desc';
}

// Setup
export interface SetupStatus {
  is_initialized: boolean;
  setup_required: boolean;
  has_admin: boolean;
}

export interface SetupInitRequest {
  username: string;
  password: string;
  email?: string;
}

export interface SetupInitResponse {
  user: UserSummary;
  tokens: AuthTokenPair;
}

// Auth
export interface LoginRequest {
  username: string;
  password: string;
}

export interface AuthTokenPair {
  access_token: string;
  refresh_token: string;
  expires_in: number;
  refresh_expires_in: number;
  token_type: string;
}

export interface LoginResponse {
  user: UserSummary;
  tokens: AuthTokenPair;
}

export interface RefreshTokenRequest {
  refresh_token: string;
}

export interface RefreshTokenResponse {
  tokens: AuthTokenPair;
}

export interface LogoutRequest {
  refresh_token: string;
}

// User
export interface UserSummary {
  id: number;
  username: string;
  email: string;
  role_key: 'super_admin' | 'admin' | 'operator' | 'user';
  status: 'active' | 'locked';
  created_at: string;
}

export interface CurrentUserResponse {
  user: UserSummary;
  capabilities: string[];
}

// Storage Source
export interface StorageSource {
  id: number;
  name: string;
  driver_type: 'local' | 's3' | 'pikpak' | 'onedrive';
  status: 'online' | 'offline' | 'error';
  is_enabled: boolean;
  is_webdav_exposed: boolean;
  webdav_read_only: boolean;
  webdav_slug: string;
  mount_path: string;
  root_path: string;
  used_bytes: number | null;
  total_bytes: number | null;
  created_at: string;
  updated_at: string;
}

export interface SourceDetailResponse {
  source: StorageSource;
  config: Record<string, unknown>;
  secret_fields: Record<string, { configured: boolean; masked: string }>;
  last_checked_at: string | null;
}

export interface CreateSourceRequest {
  name: string;
  driver_type: string;
  is_enabled: boolean;
  is_webdav_exposed: boolean;
  webdav_read_only: boolean;
  mount_path: string;
  root_path: string;
  sort_order?: number;
  config: Record<string, unknown>;
  secret_patch?: Record<string, string | null>;
}

export interface UpdateSourceRequest {
  name?: string;
  is_enabled?: boolean;
  is_webdav_exposed?: boolean;
  webdav_read_only?: boolean;
  mount_path?: string;
  root_path?: string;
  sort_order?: number;
  config?: Record<string, unknown>;
  secret_patch?: Record<string, string | null>;
}

export interface TestSourceResponse {
  reachable: boolean;
  status: string;
  latency_ms: number;
  checked_at: string;
  warnings: string[];
}

// File
export interface FileItem {
  name: string;
  path: string;
  parent_path: string;
  source_id: number;
  is_dir: boolean;
  size: number;
  mime_type: string;
  extension: string;
  etag: string;
  modified_at: string;
  created_at: string;
  can_preview: boolean;
  can_download: boolean;
  can_delete: boolean;
  thumbnail_url: string | null;
}

export interface PathPermissions {
  read: boolean;
  write: boolean;
  delete: boolean;
  share: boolean;
}

export interface FileListResult {
  items: FileItem[];
  current_path: string;
  current_source_id: number;
  current_permissions?: PathPermissions;
}

export interface ListFilesParams extends PaginationParams {
  source_id: number;
  path: string;
}

export interface SearchFilesParams extends PaginationParams {
  source_id: number;
  keyword: string;
  path_prefix?: string;
}

export interface MkdirRequest {
  source_id: number;
  parent_path: string;
  name: string;
}

export interface RenameRequest {
  source_id: number;
  path: string;
  new_name: string;
}

export interface MoveRequest {
  source_id: number;
  path: string;
  target_path: string;
}

export interface CopyRequest {
  source_id: number;
  path: string;
  target_path: string;
}

export interface DeleteRequest {
  source_id: number;
  path: string;
  delete_mode?: 'trash' | 'permanent';
}

export interface AccessUrlRequest {
  source_id: number;
  path: string;
  purpose?: 'preview' | 'download';
  disposition?: 'inline' | 'attachment';
  expires_in?: number;
}

export interface AccessUrlResponse {
  url: string;
  method: string;
  expires_at: string;
}

// Upload
export interface UploadSession {
  upload_id: string;
  source_id: number;
  path: string;
  filename: string;
  file_size: number;
  file_hash: string;
  chunk_size: number;
  total_chunks: number;
  uploaded_chunks: number[];
  status: 'pending' | 'uploading' | 'completed' | 'canceled' | 'expired';
  is_fast_upload: boolean;
  expires_at: string;
  target_virtual_parent_path?: string;
  resolved_source_id?: number;
  resolved_inner_parent_path?: string;
}

export interface UploadInitRequest {
  source_id?: number;
  path?: string;
  filename: string;
  file_size: number;
  file_hash: string;
  last_modified_at?: string;
  target_virtual_parent_path?: string;
}

export interface PartInstruction {
  index: number;
  method: string;
  url: string;
  headers: Record<string, string>;
  byte_range: {
    start: number;
    end: number;
  };
  expires_at: string;
}

export interface UploadTransport {
  mode: 'server_chunk' | 'direct_parts';
  driver_type: string;
  concurrency: number;
  retry_limit: number;
}

export interface UploadInitResponse {
  is_fast_upload: boolean;
  file?: FileItem;
  result_vfs_node_id?: number;
  upload?: UploadSession;
  transport?: UploadTransport;
  part_instructions?: PartInstruction[];
}

export interface UploadChunkResponse {
  upload_id: string;
  index: number;
  received_bytes: number;
  already_uploaded: boolean;
}

export interface UploadFinishRequest {
  upload_id: string;
  parts?: { index: number; etag: string }[];
}

export interface UploadFinishResponse {
  completed: boolean;
  upload_id: string;
  file: FileItem;
  result_vfs_node_id?: number;
}

// Task
export interface DownloadTask {
  id: number;
  type: 'download';
  status: 'pending' | 'running' | 'paused' | 'completed' | 'failed' | 'canceled';
  downloader_type?: 'aria2' | 'qbittorrent' | 'pikpak_native';
  source_id: number;
  save_path: string;
  target_virtual_parent_path?: string;
  display_name: string;
  source_url: string;
  progress: number;
  downloaded_bytes: number;
  total_bytes: number | null;
  speed_bytes: number;
  eta_seconds: number | null;
  error_message: string | null;
  created_at: string;
  updated_at: string;
  finished_at: string | null;
  result?: {
    file_path: string | null;
    source_id: number;
  };
  save_virtual_path?: string;
  resolved_source_id?: number;
  resolved_inner_save_path?: string;
  target_vfs_parent_node_id?: number;
  result_vfs_node_id?: number;
  target_filename?: string;
}

export interface CreateTaskRequest {
  type: 'download';
  url: string;
  source_id?: number;
  save_path?: string;
  target_virtual_parent_path?: string;
  target_filename?: string;
}

// RSS
export type RSSItemStatus =
  | 'new'
  | 'unsupported'
  | 'ignored'
  | 'matched'
  | 'enqueued'
  | 'retry_pending'
  | 'completed'
  | 'needs_attention'
  | 'failed';
export type RSSLinkType = 'magnet' | 'torrent' | 'http' | 'unsupported' | string;
export type RSSSourceHealthStatus = 'ok' | 'degraded' | 'circuit_open' | string;
export type RSSRefreshAllItemStatus = 'success' | 'failed' | 'skipped' | string;
export type RSSSubscriptionPreviewResult = 'matched' | 'missing' | 'excluded' | string;

export interface RSSAnimeParsedView {
  anime_title: string;
  season: string;
  episode: string;
  subtitle_group: string;
  resolution: string;
}

export interface RSSRefreshStatsView {
  source_id: number;
  fetched: number;
  created: number;
  updated: number;
  matched: number;
  enqueued: number;
  unsupported: number;
  failed: number;
}

export interface RSSSourceView {
  id: number;
  user_id?: number;
  name: string;
  url: string;
  is_enabled: boolean;
  refresh_interval_seconds: number;
  last_refreshed_at: string | null;
  last_error: string | null;
  health_status: RSSSourceHealthStatus;
  consecutive_failures: number;
  last_success_at: string | null;
  next_refresh_at: string | null;
  last_refresh_status: string;
  last_refresh_stats: RSSRefreshStatsView | null;
  created_at: string;
  updated_at: string;
}

export interface RSSSourceUpsertRequest {
  name: string;
  url: string;
  is_enabled?: boolean;
  refresh_interval_seconds: number;
}

export interface RSSRefreshResponse {
  source_id: number;
  fetched: number;
  created: number;
  updated: number;
  matched: number;
  enqueued: number;
  unsupported: number;
  failed: number;
}

export interface RSSRefreshAllItem {
  source_id: number;
  status: RSSRefreshAllItemStatus;
  error?: string | null;
  stats?: RSSRefreshResponse | null;
}

export interface RSSRefreshAllResponse {
  items: RSSRefreshAllItem[];
  refreshed: number;
  skipped: number;
  failed: number;
}

export interface RSSSubscriptionView {
  id: number;
  user_id?: number;
  source_id: number;
  name: string;
  is_enabled: boolean;
  must_contain: string[];
  must_not_contain: string[];
  use_regex: boolean;
  case_sensitive: boolean;
  target_virtual_parent_path: string;
  directory_template: string;
  filename_template: string;
  resolved_source_id?: number;
  resolved_inner_parent_path?: string;
  created_at: string;
  updated_at: string;
}

export interface RSSSubscriptionUpsertRequest {
  source_id: number;
  name: string;
  is_enabled?: boolean;
  must_contain: string[];
  must_not_contain: string[];
  use_regex: boolean;
  case_sensitive: boolean;
  target_virtual_parent_path: string;
  directory_template?: string;
  filename_template?: string;
}

export interface RSSSubscriptionPreviewRequest {
  source_id: number;
  must_contain: string[];
  must_not_contain: string[];
  use_regex: boolean;
  case_sensitive: boolean;
}

export interface RSSSubscriptionCloneRequest {
  name?: string;
  is_enabled?: boolean;
}

export interface RSSSubscriptionBatchStateRequest {
  subscription_ids: number[];
  is_enabled: boolean;
}

export interface RSSSubscriptionBatchStateResponse {
  items: Array<{
    subscription_id: number;
    success: boolean;
    subscription?: RSSSubscriptionView;
    error_code?: string;
    error_message?: string;
  }>;
  succeeded: number;
  failed: number;
}

export interface RSSSubscriptionPreviewItem {
  item_id: number;
  title: string;
  download_url: string;
  current_status: RSSItemStatus | string;
  result: RSSSubscriptionPreviewResult;
  matched: string[];
  missing: string[];
  excluded: string[];
}

export interface RSSSubscriptionPreviewResponse {
  subscription_id: number;
  source_id: number;
  items: RSSSubscriptionPreviewItem[];
  matched: number;
  missing: number;
  excluded: number;
}

export interface RSSItemView {
  id: number;
  user_id?: number;
  source_id: number;
  title: string;
  link: string;
  published_at: string | null;
  guid: string;
  download_url: string;
  link_type: RSSLinkType;
  status: RSSItemStatus;
  matched_subscription_id: number | null;
  task_id: number | null;
  result_vfs_node_id?: number;
  error_message: string | null;
  retry_count: number;
  max_retry_count: number;
  last_attempt_at: string | null;
  next_retry_at: string | null;
  retry_reason: string | null;
  parsed: RSSAnimeParsedView;
  created_at: string;
  updated_at: string;
}

export interface RSSItemBatchActionResponse {
  items: Array<{
    item_id: number;
    success: boolean;
    item?: RSSItemView;
    error_code?: string;
    error_message?: string;
  }>;
  succeeded: number;
  failed: number;
}

export interface RSSExportSource {
  name: string;
  url: string;
  is_enabled: boolean;
  refresh_interval_seconds: number;
}

export interface RSSExportSubscription {
  source_url: string;
  name: string;
  is_enabled: boolean;
  must_contain: string[];
  must_not_contain: string[];
  use_regex: boolean;
  case_sensitive: boolean;
  target_virtual_parent_path: string;
  directory_template: string;
  filename_template: string;
  source_ref?: string;
}

export interface RSSExportResponse {
  version: number;
  exported_at: string;
  sources: RSSExportSource[];
  subscriptions: RSSExportSubscription[];
}

export interface RSSImportRequest {
  dry_run: boolean;
  sources: RSSExportSource[];
  subscriptions: RSSExportSubscription[];
}

export interface RSSImportSectionResult {
  items: Array<{
    index: number;
    action: 'create' | 'reuse' | 'skip' | 'failed' | string;
    success: boolean;
    id?: number;
    source_url?: string;
    name?: string;
    error_code?: string;
    error_message?: string;
  }>;
  created: number;
  reused: number;
  skipped: number;
  failed: number;
}

export interface RSSImportResponse {
  dry_run: boolean;
  sources: RSSImportSectionResult;
  subscriptions: RSSImportSectionResult;
}

export interface RSSQBitHealthResponse {
  enabled: boolean;
  status: 'disabled' | 'ok' | 'unavailable' | string;
  error?: string | null;
}

export type NotificationEventType =
  | 'rss.source_failure'
  | 'rss.item_needs_attention'
  | 'rss.download_completed'
  | string;

export type NotificationEventStatus =
  | 'pending'
  | 'delivered'
  | 'retry_pending'
  | 'failed'
  | 'skipped'
  | string;

export interface NotificationChannelConfigView {
  url: string;
  secret_configured: boolean;
}

export interface NotificationChannelView {
  id: number;
  name: string;
  type: 'webhook' | string;
  is_enabled: boolean;
  event_types: NotificationEventType[];
  config: NotificationChannelConfigView;
  created_at: string;
  updated_at: string;
}

export interface NotificationChannelUpsertRequest {
  name: string;
  type: 'webhook';
  is_enabled?: boolean;
  event_types?: NotificationEventType[];
  config: {
    url: string;
    secret?: string;
  };
}

export interface NotificationEventView {
  id: number;
  user_id?: number;
  event_type: NotificationEventType;
  severity: 'info' | 'warning' | 'error' | string;
  title: string;
  message: string;
  payload: Record<string, unknown>;
  status: NotificationEventStatus;
  attempts: number;
  max_attempts: number;
  last_attempt_at: string | null;
  next_attempt_at: string | null;
  delivered_at: string | null;
  last_error: string | null;
  created_at: string;
  updated_at: string;
}

// System
export interface SystemConfigPublic {
  site_name: string;
  multi_user_enabled: boolean;
  default_source_id: number;
  max_upload_size: number;
  default_chunk_size: number;
  webdav_enabled: boolean;
  webdav_prefix: string;
  theme: 'light' | 'dark' | 'system';
  language: string;
  time_zone: string;
}

export interface SystemVersion {
  service: string;
  version: string;
  commit: string | null;
  build_time: string | null;
  go_version: string | null;
  api_version: string;
}

export interface HealthStatus {
  status: string;
  service: string;
  version: string;
}

// Trash
export interface TrashItem {
  id: number;
  source_id: number;
  path: string;
  name: string;
  is_dir: boolean;
  size: number;
  deleted_at: string;
  created_at: string;
  original_virtual_path?: string;
}

// Share
export interface Share {
  id: number;
  source_id: number | null;
  path: string;
  name: string;
  is_dir: boolean;
  link: string;
  has_password: boolean;
  expires_at: string | null;
  created_at: string;
  target_vfs_node_id?: number;
  target_virtual_path?: string;
  resolved_source_id?: number;
  resolved_inner_path?: string;
}

export interface CreateShareRequest {
  vfs_node_id?: number;
  source_id?: number;
  path?: string;
  expires_in?: number;
  password?: string;
}

// User Management
export interface User {
  id: number;
  username: string;
  email: string;
  role_key: 'super_admin' | 'admin' | 'operator' | 'user';
  status: 'active' | 'locked';
  created_at: string;
}

export interface CreateUserRequest {
  username: string;
  password: string;
  email?: string;
  role_key: string;
}

export interface UpdateUserRequest {
  email?: string;
  role_key?: string;
  status?: 'active' | 'locked';
}

// ACL
export interface AclRule {
  id: number;
  source_id: number | null;
  path: string;
  vfs_node_id?: number;
  virtual_path?: string;
  subject_type: 'user' | 'role';
  subject_id: number;
  effect: 'allow' | 'deny';
  priority: number;
  permissions: {
    read: boolean;
    write: boolean;
    delete: boolean;
    share: boolean;
  };
  inherit_to_children: boolean;
}

export interface CreateAclRuleRequest {
  source_id?: number;
  path?: string;
  vfs_node_id?: number;
  subject_type: string;
  subject_id: number;
  effect: string;
  priority: number;
  permissions: {
    read: boolean;
    write: boolean;
    delete: boolean;
    share: boolean;
  };
  inherit_to_children: boolean;
}

// V2 Virtual Filesystem
export type VFSSyncState =
  | 'indexed'
  | 'pending'
  | 'syncing'
  | 'stale'
  | 'missing'
  | 'conflict'
  | 'error';

export interface VFSTag {
  id: number;
  name: string;
  color: string;
  created_at?: string;
  updated_at?: string;
}

export interface VFSRefreshResponse {
  path: string;
  node_id?: number;
  seen: number;
  indexed: number;
  updated: number;
  missing: number;
  conflicts: number;
  errors: number;
  sync_state?: VFSSyncState;
  error?: string | null;
}

export interface VFSItem {
  id?: number;
  name: string;
  path: string;
  parent_path: string;
  source_id: number | null;
  entry_kind: 'file' | 'directory';
  is_virtual: boolean;
  is_mount_point: boolean;
  size: number;
  mime_type: string;
  extension: string;
  modified_at: string;
  created_at: string;
  etag: string;
  can_preview: boolean;
  can_download: boolean;
  can_delete: boolean;
  sync_state?: VFSSyncState;
  thumbnail_url: string | null;
}

export interface VFSListResult {
  items: VFSItem[];
  current_path: string;
  current_permissions?: PathPermissions;
}

export interface VFSAccessUrlRequest {
  path: string;
  purpose?: 'preview' | 'download';
  disposition?: 'inline' | 'attachment';
  expires_in?: number;
}

// System
export interface SystemStats {
  sources_total: number;
  files_total: number;
  downloads_running: number;
  downloads_completed: number;
  users_total: number;
  storage_used_bytes: number;
}

// Audit
export interface AuditLog {
  id: number;
  occurred_at: string;
  actor: {
    user_id: number;
    username: string;
    role_key: string;
  };
  request: {
    request_id: string;
    entrypoint: string;
    client_ip: string;
    user_agent: string;
    method: string;
    path: string;
  };
  target: {
    source_id: number;
    virtual_path: string;
  };
  resource_type: string;
  action: string;
  result: string;
  summary: string;
  before?: Record<string, unknown>;
  after?: Record<string, unknown>;
  detail?: Record<string, unknown>;
}

export interface AuditLogListResponse {
  items: AuditLog[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
}
