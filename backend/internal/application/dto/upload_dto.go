package dto

// UploadSessionView 表示上传会话视图。
type UploadSessionView struct {
	UploadID                string `json:"upload_id"`
	SourceID                uint   `json:"source_id"`
	Path                    string `json:"path"`
	Filename                string `json:"filename"`
	FileSize                int64  `json:"file_size"`
	FileHash                string `json:"file_hash"`
	ChunkSize               int64  `json:"chunk_size"`
	TotalChunks             int    `json:"total_chunks"`
	UploadedChunks          []int  `json:"uploaded_chunks"`
	Status                  string `json:"status"`
	IsFastUpload            bool   `json:"is_fast_upload"`
	ExpiresAt               string `json:"expires_at"`
	TargetVirtualParentPath string `json:"target_virtual_parent_path,omitempty"`
	ResolvedSourceID        uint   `json:"resolved_source_id,omitempty"`
	ResolvedInnerParentPath string `json:"resolved_inner_parent_path,omitempty"`
}

// UploadTransport 表示上传传输方式。
type UploadTransport struct {
	Mode        string `json:"mode"`
	DriverType  string `json:"driver_type"`
	Concurrency int    `json:"concurrency"`
	RetryLimit  int    `json:"retry_limit"`
}

// UploadPartInstruction 表示直传分片说明。
type UploadPartInstruction struct {
	Index     int               `json:"index"`
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	ByteRange struct {
		Start int64 `json:"start"`
		End   int64 `json:"end"`
	} `json:"byte_range"`
	ExpiresAt string `json:"expires_at"`
}

// UploadInitRequest 表示上传初始化请求。
type UploadInitRequest struct {
	SourceID                uint   `json:"source_id"`
	Path                    string `json:"path"`
	TargetVirtualParentPath string `json:"target_virtual_parent_path"`
	Filename                string `json:"filename" binding:"required"`
	FileSize                int64  `json:"file_size" binding:"required,gt=0"`
	FileHash                string `json:"file_hash"`
	LastModifiedAt          string `json:"last_modified_at"`
}

// UploadInitResponse 表示上传初始化响应。
type UploadInitResponse struct {
	IsFastUpload     bool                    `json:"is_fast_upload"`
	File             *FileItem               `json:"file,omitempty"`
	Upload           *UploadSessionView      `json:"upload,omitempty"`
	Transport        *UploadTransport        `json:"transport,omitempty"`
	PartInstructions []UploadPartInstruction `json:"part_instructions"`
}

// UploadChunkResponse 表示上传分片响应。
type UploadChunkResponse struct {
	UploadID        string `json:"upload_id"`
	Index           int    `json:"index"`
	ReceivedBytes   int64  `json:"received_bytes"`
	AlreadyUploaded bool   `json:"already_uploaded"`
}

// UploadFinishRequest 表示上传完成请求。
type UploadFinishRequest struct {
	UploadID string           `json:"upload_id" binding:"required"`
	Parts    []UploadPartETag `json:"parts"`
}

// UploadPartETag 表示直传场景 part etag。
type UploadPartETag struct {
	Index int    `json:"index"`
	ETag  string `json:"etag"`
}

// UploadFinishResponse 表示上传完成响应。
type UploadFinishResponse struct {
	Completed bool     `json:"completed"`
	UploadID  string   `json:"upload_id"`
	File      FileItem `json:"file"`
}

// UploadSessionListResponse 表示上传会话列表响应。
type UploadSessionListResponse struct {
	Items []UploadSessionView `json:"items"`
}
