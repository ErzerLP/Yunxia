package entity

import "time"

// TrashItem 表示回收站元数据。
type TrashItem struct {
	ID                  uint
	SourceID            uint
	OriginalPath        string
	OriginalVirtualPath string
	TrashPath           string
	Name                string
	IsDir               bool
	Size                int64
	DeletedAt           time.Time
	ExpiresAt           time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}
