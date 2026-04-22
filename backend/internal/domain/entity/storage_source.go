package entity

import "time"

// StorageSource 表示存储源配置。
type StorageSource struct {
	ID              uint
	Name            string
	DriverType      string
	Status          string
	IsEnabled       bool
	IsWebDAVExposed bool
	WebDAVReadOnly  bool
	WebDAVSlug      string
	MountPath       string
	RootPath        string
	SortOrder       int
	ConfigJSON      string
	LastCheckedAt   *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
