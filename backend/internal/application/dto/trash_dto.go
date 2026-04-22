package dto

// TrashListQuery 表示回收站列表查询参数。
type TrashListQuery struct {
	SourceID uint `form:"source_id" binding:"required"`
	Page     int  `form:"page"`
	PageSize int  `form:"page_size"`
}

// TrashItemView 表示回收站项。
type TrashItemView struct {
	ID           uint   `json:"id"`
	SourceID     uint   `json:"source_id"`
	OriginalPath string `json:"original_path"`
	TrashPath    string `json:"trash_path"`
	Name         string `json:"name"`
	Size         int64  `json:"size"`
	DeletedAt    string `json:"deleted_at"`
	ExpiresAt    string `json:"expires_at"`
}

// TrashListResponse 表示回收站列表响应。
type TrashListResponse struct {
	Items []TrashItemView `json:"items"`
}

// TrashRestoreResponse 表示回收站恢复响应。
type TrashRestoreResponse struct {
	ID           uint   `json:"id"`
	Restored     bool   `json:"restored"`
	RestoredPath string `json:"restored_path"`
}

// TrashDeleteResponse 表示回收站删除响应。
type TrashDeleteResponse struct {
	ID           *uint `json:"id,omitempty"`
	Deleted      bool  `json:"deleted,omitempty"`
	SourceID     *uint `json:"source_id,omitempty"`
	Cleared      bool  `json:"cleared,omitempty"`
	DeletedCount int   `json:"deleted_count,omitempty"`
}
