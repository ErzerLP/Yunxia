package dto

// VFSItem 表示统一虚拟目录树中的条目。
type VFSItem struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	ParentPath   string `json:"parent_path"`
	SourceID     *uint  `json:"source_id,omitempty"`
	EntryKind    string `json:"entry_kind"`
	IsVirtual    bool   `json:"is_virtual"`
	IsMountPoint bool   `json:"is_mount_point"`
	Size         int64  `json:"size"`
	MimeType     string `json:"mime_type"`
	Extension    string `json:"extension"`
	ModifiedAt   string `json:"modified_at"`
	CreatedAt    string `json:"created_at"`
	Etag         string `json:"etag"`
	CanPreview   bool   `json:"can_preview"`
	CanDownload  bool   `json:"can_download"`
	CanDelete    bool   `json:"can_delete"`
}

// VFSListResponse 表示统一虚拟目录列表响应。
type VFSListResponse struct {
	Items       []VFSItem `json:"items"`
	CurrentPath string    `json:"current_path"`
}

// VFSSearchResponse 表示统一虚拟目录搜索响应。
type VFSSearchResponse struct {
	Items      []VFSItem `json:"items"`
	PathPrefix string    `json:"path_prefix"`
	Keyword    string    `json:"keyword"`
}

// VFSListQuery 表示统一虚拟目录列表查询参数。
type VFSListQuery struct {
	Path string `form:"path"`
}

// VFSSearchQuery 表示统一虚拟目录搜索查询参数。
type VFSSearchQuery struct {
	Path     string `form:"path"`
	Keyword  string `form:"keyword" binding:"required"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

// VFSAccessURLRequest 表示统一虚拟目录访问地址请求。
type VFSAccessURLRequest struct {
	Path        string `json:"path" binding:"required"`
	Purpose     string `json:"purpose" binding:"required"`
	Disposition string `json:"disposition" binding:"required"`
	ExpiresIn   int    `json:"expires_in"`
}
