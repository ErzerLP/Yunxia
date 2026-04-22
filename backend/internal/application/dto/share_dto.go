package dto

// CreateShareRequest 表示创建分享请求。
type CreateShareRequest struct {
	SourceID  uint   `json:"source_id" binding:"required"`
	Path      string `json:"path" binding:"required"`
	ExpiresIn int64  `json:"expires_in"`
	Password  string `json:"password"`
}

// UpdateShareRequest 表示更新分享请求。
type UpdateShareRequest struct {
	ExpiresIn *int64  `json:"expires_in"`
	Password  *string `json:"password"`
}

// ShareView 表示分享链接视图。
type ShareView struct {
	ID          uint    `json:"id"`
	SourceID    uint    `json:"source_id"`
	Path        string  `json:"path"`
	Name        string  `json:"name"`
	IsDir       bool    `json:"is_dir"`
	Link        string  `json:"link"`
	HasPassword bool    `json:"has_password"`
	ExpiresAt   *string `json:"expires_at"`
	CreatedAt   string  `json:"created_at"`
}

// ShareListResponse 表示分享列表响应。
type ShareListResponse struct {
	Items []ShareView `json:"items"`
}

// PublicShareEntry 表示公开分享目录中的条目。
type PublicShareEntry struct {
	Name         string  `json:"name"`
	Path         string  `json:"path"`
	ParentPath   string  `json:"parent_path"`
	IsDir        bool    `json:"is_dir"`
	PreviewType  string  `json:"preview_type"`
	Size         int64   `json:"size"`
	MimeType     string  `json:"mime_type"`
	Extension    string  `json:"extension"`
	ModifiedAt   string  `json:"modified_at"`
	CreatedAt    string  `json:"created_at"`
	CanPreview   bool    `json:"can_preview"`
	CanDownload  bool    `json:"can_download"`
	ThumbnailURL *string `json:"thumbnail_url"`
}

// PublicShareCurrentDir 表示当前公开浏览目录信息。
type PublicShareCurrentDir struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	ParentPath string `json:"parent_path"`
	IsRoot     bool   `json:"is_root"`
}

// PublicShareBreadcrumb 表示公开目录面包屑。
type PublicShareBreadcrumb struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// PublicSharePagination 表示公开目录分页信息。
type PublicSharePagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// PublicShareOpenResponse 表示公开分享访问结果。
type PublicShareOpenResponse struct {
	Share       ShareView               `json:"share"`
	CurrentPath string                  `json:"current_path"`
	CurrentDir  PublicShareCurrentDir   `json:"current_dir"`
	Breadcrumbs []PublicShareBreadcrumb `json:"breadcrumbs"`
	Pagination  PublicSharePagination   `json:"pagination"`
	Items       []PublicShareEntry      `json:"items"`
}

// DeleteShareResponse 表示删除分享响应。
type DeleteShareResponse struct {
	ID      uint `json:"id"`
	Deleted bool `json:"deleted"`
}
