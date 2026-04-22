package entity

import "time"

// ShareLink 表示对外公开的文件分享链接。
type ShareLink struct {
	ID                uint
	UserID            uint
	SourceID          uint
	Path              string
	TargetVirtualPath string
	ResolvedSourceID  uint
	ResolvedInnerPath string
	Name              string
	IsDir             bool
	Token             string
	PasswordHash      *string
	ExpiresAt         *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
