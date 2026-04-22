package dto

// FileItem 表示文件或目录项。
type FileItem struct {
	Name         string  `json:"name"`
	Path         string  `json:"path"`
	ParentPath   string  `json:"parent_path"`
	SourceID     uint    `json:"source_id"`
	IsDir        bool    `json:"is_dir"`
	Size         int64   `json:"size"`
	MimeType     string  `json:"mime_type"`
	Extension    string  `json:"extension"`
	Etag         string  `json:"etag"`
	ModifiedAt   string  `json:"modified_at"`
	CreatedAt    string  `json:"created_at"`
	CanPreview   bool    `json:"can_preview"`
	CanDownload  bool    `json:"can_download"`
	CanDelete    bool    `json:"can_delete"`
	ThumbnailURL *string `json:"thumbnail_url"`
}

// FileListResponse 表示文件列表响应。
type FileListResponse struct {
	Items           []FileItem `json:"items"`
	CurrentPath     string     `json:"current_path"`
	CurrentSourceID uint       `json:"current_source_id"`
}

// FileSearchResponse 表示文件搜索响应。
type FileSearchResponse struct {
	Items           []FileItem `json:"items"`
	Keyword         string     `json:"keyword"`
	CurrentSourceID uint       `json:"current_source_id"`
	PathPrefix      *string    `json:"path_prefix"`
}

// FileListQuery 表示文件列表查询参数。
type FileListQuery struct {
	SourceID  uint   `form:"source_id" binding:"required"`
	Path      string `form:"path" binding:"required"`
	Page      int    `form:"page"`
	PageSize  int    `form:"page_size"`
	SortBy    string `form:"sort_by"`
	SortOrder string `form:"sort_order"`
}

// FileSearchQuery 表示文件搜索查询参数。
type FileSearchQuery struct {
	SourceID   uint   `form:"source_id" binding:"required"`
	Keyword    string `form:"keyword" binding:"required"`
	PathPrefix string `form:"path_prefix"`
	Page       int    `form:"page"`
	PageSize   int    `form:"page_size"`
}

// MkdirRequest 表示创建目录请求。
type MkdirRequest struct {
	SourceID   uint   `json:"source_id" binding:"required"`
	ParentPath string `json:"parent_path" binding:"required"`
	Name       string `json:"name" binding:"required"`
}

// RenameRequest 表示重命名请求。
type RenameRequest struct {
	SourceID uint   `json:"source_id" binding:"required"`
	Path     string `json:"path" binding:"required"`
	NewName  string `json:"new_name" binding:"required"`
}

// MoveCopyRequest 表示移动/复制请求。
type MoveCopyRequest struct {
	SourceID   uint   `json:"source_id" binding:"required"`
	Path       string `json:"path" binding:"required"`
	TargetPath string `json:"target_path" binding:"required"`
}

// DeleteFileRequest 表示删除文件请求。
type DeleteFileRequest struct {
	SourceID   uint   `json:"source_id" binding:"required"`
	Path       string `json:"path" binding:"required"`
	DeleteMode string `json:"delete_mode"`
}

// AccessURLRequest 表示短时访问地址请求。
type AccessURLRequest struct {
	SourceID    uint   `json:"source_id" binding:"required"`
	Path        string `json:"path" binding:"required"`
	Purpose     string `json:"purpose" binding:"required"`
	Disposition string `json:"disposition" binding:"required"`
	ExpiresIn   int    `json:"expires_in"`
}

// AccessURLResponse 表示短时访问地址响应。
type AccessURLResponse struct {
	URL       string `json:"url"`
	Method    string `json:"method"`
	ExpiresAt string `json:"expires_at"`
}
