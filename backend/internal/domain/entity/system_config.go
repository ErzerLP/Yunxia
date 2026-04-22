package entity

import "time"

// SystemConfig 表示前端可见的系统配置。
type SystemConfig struct {
	ID               uint
	SiteName         string
	MultiUserEnabled bool
	DefaultSourceID  *uint
	MaxUploadSize    int64
	DefaultChunkSize int64
	WebDAVEnabled    bool
	WebDAVPrefix     string
	Theme            string
	Language         string
	TimeZone         string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
