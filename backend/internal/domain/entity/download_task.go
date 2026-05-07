package entity

import "time"

// DownloadTask 表示离线下载任务。
type DownloadTask struct {
	ID                      uint
	UserID                  uint
	Type                    string
	DownloaderType          string
	Status                  string
	SourceID                uint
	SavePath                string
	TargetVFSParentNodeID   uint
	TargetVirtualParentPath string
	TargetFilename          string
	SaveVirtualPath         string
	ResolvedSourceID        uint
	ResolvedInnerSavePath   string
	ResultVFSNodeID         uint
	StagingDir              string
	DisplayName             string
	SourceURL               string
	ExternalID              string
	Progress                float64
	DownloadedBytes         int64
	TotalBytes              *int64
	SpeedBytes              int64
	ETASeconds              *int64
	ErrorMessage            *string
	FinishedAt              *time.Time
	CreatedAt               time.Time
	UpdatedAt               time.Time
}
