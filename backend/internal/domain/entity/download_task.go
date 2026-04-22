package entity

import "time"

// DownloadTask 表示离线下载任务。
type DownloadTask struct {
	ID                    uint
	UserID                uint
	Type                  string
	Status                string
	SourceID              uint
	SavePath              string
	SaveVirtualPath       string
	ResolvedSourceID      uint
	ResolvedInnerSavePath string
	DisplayName           string
	SourceURL             string
	ExternalID            string
	Progress              float64
	DownloadedBytes       int64
	TotalBytes            *int64
	SpeedBytes            int64
	ETASeconds            *int64
	ErrorMessage          *string
	FinishedAt            *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}
