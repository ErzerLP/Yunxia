package dto

// VFSTagView 表示 VFS 控制面标签。
type VFSTagView struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Color     string `json:"color"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// VFSTagListResponse 表示标签列表响应。
type VFSTagListResponse struct {
	Items []VFSTagView `json:"items"`
}

// VFSTagUpsertRequest 表示创建 / 更新标签请求。
type VFSTagUpsertRequest struct {
	Name  string `json:"name" binding:"required"`
	Color string `json:"color"`
}

// VFSNodeTagRequest 表示 VFS 节点绑定 / 解绑标签请求。
type VFSNodeTagRequest struct {
	Path  string `json:"path" binding:"required"`
	TagID uint   `json:"tag_id" binding:"required"`
}

// VFSNodeTagListResponse 表示指定 VFS 节点的标签响应。
type VFSNodeTagListResponse struct {
	Path string       `json:"path"`
	Tags []VFSTagView `json:"tags"`
}
