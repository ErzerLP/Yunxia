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
  driver_type: 'local' | 's3' | 'onedrive';
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

export interface FileListResult {
  items: FileItem[];
  current_path: string;
  current_source_id: number;
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
}

export interface UploadInitRequest {
  source_id?: number;
  path?: string;
  filename: string;
  file_size: number;
  file_hash: string;
  last_modified_at?: string;
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
}

// Task
export interface DownloadTask {
  id: number;
  type: 'download';
  status: 'pending' | 'running' | 'paused' | 'completed' | 'failed' | 'canceled';
  source_id: number;
  save_path: string;
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
}

export interface CreateTaskRequest {
  type: 'download';
  url: string;
  source_id?: number;
  save_path?: string;
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
}

// Share
export interface Share {
  id: number;
  source_id: number;
  path: string;
  name: string;
  is_dir: boolean;
  link: string;
  has_password: boolean;
  expires_at: string | null;
  created_at: string;
}

export interface CreateShareRequest {
  source_id: number;
  path: string;
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
  source_id: number;
  path: string;
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
  source_id: number;
  path: string;
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
