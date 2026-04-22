package dto

// StorageSourceView 表示前端可见的存储源。
type StorageSourceView struct {
	ID              uint   `json:"id"`
	Name            string `json:"name"`
	DriverType      string `json:"driver_type"`
	Status          string `json:"status"`
	IsEnabled       bool   `json:"is_enabled"`
	IsWebDAVExposed bool   `json:"is_webdav_exposed"`
	WebDAVReadOnly  bool   `json:"webdav_read_only"`
	WebDAVSlug      string `json:"webdav_slug"`
	RootPath        string `json:"root_path"`
	UsedBytes       *int64 `json:"used_bytes"`
	TotalBytes      *int64 `json:"total_bytes"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// SecretFieldMask 表示敏感字段掩码状态。
type SecretFieldMask struct {
	Configured bool   `json:"configured"`
	Masked     string `json:"masked"`
}

// SourceListResponse 表示存储源列表响应。
type SourceListResponse struct {
	Items []StorageSourceView `json:"items"`
	View  string              `json:"view"`
}

// SourceDetailResponse 表示存储源详情响应。
type SourceDetailResponse struct {
	Source        StorageSourceView             `json:"source"`
	Config        map[string]any                `json:"config"`
	SecretFields  map[string]SecretFieldMask    `json:"secret_fields"`
	LastCheckedAt *string                       `json:"last_checked_at"`
}

// SourceUpsertRequest 表示创建/更新存储源请求。
type SourceUpsertRequest struct {
	Name            string         `json:"name" binding:"required"`
	DriverType      string         `json:"driver_type"`
	IsEnabled       bool           `json:"is_enabled"`
	IsWebDAVExposed bool           `json:"is_webdav_exposed"`
	WebDAVReadOnly  bool           `json:"webdav_read_only"`
	RootPath        string         `json:"root_path" binding:"required"`
	SortOrder       int            `json:"sort_order"`
	Config          map[string]any `json:"config"`
	SecretPatch     map[string]any `json:"secret_patch"`
}

// SourceTestResponse 表示测试存储源结果。
type SourceTestResponse struct {
	Reachable bool     `json:"reachable"`
	Status    string   `json:"status"`
	LatencyMS int64    `json:"latency_ms"`
	CheckedAt string   `json:"checked_at"`
	Warnings  []string `json:"warnings"`
}
