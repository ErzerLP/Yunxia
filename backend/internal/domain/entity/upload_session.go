package entity

import "time"

// UploadSession 表示上传会话。
type UploadSession struct {
	UploadID                string
	UserID                  uint
	SourceID                uint
	Path                    string
	TargetVFSParentNodeID   uint
	TargetVirtualParentPath string
	ResolvedSourceID        uint
	ResolvedInnerParentPath string
	ResultVFSNodeID         uint
	Filename                string
	FileSize                int64
	FileHash                string
	ChunkSize               int64
	TotalChunks             int
	UploadedChunks          []int
	StorageDataJSON         string
	Status                  string
	IsFastUpload            bool
	ExpiresAt               time.Time
	CreatedAt               time.Time
	UpdatedAt               time.Time
}
