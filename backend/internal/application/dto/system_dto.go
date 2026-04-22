package dto

// SystemConfigPublic 表示对前端公开的系统配置。
type SystemConfigPublic struct {
	SiteName         string `json:"site_name"`
	MultiUserEnabled bool   `json:"multi_user_enabled"`
	DefaultSourceID  *uint  `json:"default_source_id"`
	MaxUploadSize    int64  `json:"max_upload_size"`
	DefaultChunkSize int64  `json:"default_chunk_size"`
	WebDAVEnabled    bool   `json:"webdav_enabled"`
	WebDAVPrefix     string `json:"webdav_prefix"`
	Theme            string `json:"theme"`
	Language         string `json:"language"`
	TimeZone         string `json:"time_zone"`
}

// UpdateSystemConfigRequest 表示系统配置更新请求。
type UpdateSystemConfigRequest struct {
	SiteName         string `json:"site_name" binding:"required"`
	MultiUserEnabled bool   `json:"multi_user_enabled"`
	DefaultSourceID  *uint  `json:"default_source_id"`
	MaxUploadSize    int64  `json:"max_upload_size" binding:"required,gt=0"`
	DefaultChunkSize int64  `json:"default_chunk_size" binding:"required,gt=0"`
	WebDAVEnabled    bool   `json:"webdav_enabled"`
	WebDAVPrefix     string `json:"webdav_prefix" binding:"required"`
	Theme            string `json:"theme" binding:"required"`
	Language         string `json:"language" binding:"required"`
	TimeZone         string `json:"time_zone" binding:"required"`
}

// VersionResponse 表示版本信息。
type VersionResponse struct {
	Service    string  `json:"service"`
	Version    string  `json:"version"`
	Commit     *string `json:"commit"`
	BuildTime  *string `json:"build_time"`
	GoVersion  string  `json:"go_version"`
	APIVersion string  `json:"api_version"`
}

// SystemStatsResponse 表示系统统计信息。
type SystemStatsResponse struct {
	SourcesTotal       int64 `json:"sources_total"`
	FilesTotal         int64 `json:"files_total"`
	DownloadsRunning   int64 `json:"downloads_running"`
	DownloadsCompleted int64 `json:"downloads_completed"`
	UsersTotal         int64 `json:"users_total"`
	StorageUsedBytes   int64 `json:"storage_used_bytes"`
}
