package dto

// DownloadTaskView 表示下载任务视图。
type DownloadTaskView struct {
	ID                    uint    `json:"id"`
	Type                  string  `json:"type"`
	Status                string  `json:"status"`
	SourceID              uint    `json:"source_id"`
	SavePath              string  `json:"save_path"`
	SaveVirtualPath       string  `json:"save_virtual_path,omitempty"`
	ResolvedSourceID      uint    `json:"resolved_source_id,omitempty"`
	ResolvedInnerSavePath string  `json:"resolved_inner_save_path,omitempty"`
	DisplayName           string  `json:"display_name"`
	SourceURL             string  `json:"source_url"`
	Progress              float64 `json:"progress"`
	DownloadedBytes       int64   `json:"downloaded_bytes"`
	TotalBytes            *int64  `json:"total_bytes"`
	SpeedBytes            int64   `json:"speed_bytes"`
	ETASeconds            *int64  `json:"eta_seconds"`
	ErrorMessage          *string `json:"error_message"`
	CreatedAt             string  `json:"created_at"`
	UpdatedAt             string  `json:"updated_at"`
	FinishedAt            *string `json:"finished_at"`
}

// TaskListResponse 表示任务列表响应。
type TaskListResponse struct {
	Items []DownloadTaskView `json:"items"`
}

// CreateTaskRequest 表示创建任务请求。
type CreateTaskRequest struct {
	Type     string `json:"type" binding:"required"`
	URL      string `json:"url" binding:"required"`
	SourceID uint   `json:"source_id" binding:"required"`
	SavePath string `json:"save_path" binding:"required"`
}

// CancelTaskResponse 表示取消任务响应。
type CancelTaskResponse struct {
	ID         uint `json:"id"`
	Canceled   bool `json:"canceled"`
	DeleteFile bool `json:"delete_file"`
}

// TaskActionResponse 表示任务动作结果。
type TaskActionResponse struct {
	ID     uint   `json:"id"`
	Status string `json:"status"`
}
