package gorm

import "time"

// UserModel 表示用户表。
type UserModel struct {
	ID           uint      `gorm:"primaryKey"`
	Username     string    `gorm:"uniqueIndex;size:64;not null"`
	Email        string    `gorm:"size:128"`
	PasswordHash string    `gorm:"size:255;not null"`
	RoleKey      string    `gorm:"column:role_key;size:32;not null"`
	Status       string    `gorm:"column:status;size:16;not null"`
	TokenVersion int       `gorm:"not null;default:0"`
	CreatedAt    time.Time `gorm:"not null"`
	UpdatedAt    time.Time `gorm:"not null"`
}

// SystemConfigModel 表示系统配置表。
type SystemConfigModel struct {
	ID               uint   `gorm:"primaryKey"`
	SiteName         string `gorm:"size:128;not null"`
	MultiUserEnabled bool   `gorm:"not null;default:false"`
	DefaultSourceID  *uint
	MaxUploadSize    int64     `gorm:"not null"`
	DefaultChunkSize int64     `gorm:"not null"`
	WebDAVEnabled    bool      `gorm:"not null;default:true"`
	WebDAVPrefix     string    `gorm:"size:64;not null"`
	Theme            string    `gorm:"size:32;not null"`
	Language         string    `gorm:"size:32;not null"`
	TimeZone         string    `gorm:"size:64;not null"`
	CreatedAt        time.Time `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`
}

// RefreshTokenModel 表示刷新令牌表。
type RefreshTokenModel struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"index;not null"`
	TokenHash string    `gorm:"uniqueIndex;size:255;not null"`
	ExpiresAt time.Time `gorm:"not null"`
	RevokedAt *time.Time
	CreatedAt time.Time `gorm:"not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

// StorageSourceModel 表示存储源表。
type StorageSourceModel struct {
	ID              uint   `gorm:"primaryKey"`
	Name            string `gorm:"uniqueIndex;size:128;not null"`
	DriverType      string `gorm:"size:32;not null"`
	Status          string `gorm:"size:32;not null"`
	IsEnabled       bool   `gorm:"not null;default:true"`
	IsWebDAVExposed bool   `gorm:"not null;default:false"`
	WebDAVReadOnly  bool   `gorm:"not null;default:true"`
	WebDAVSlug      string `gorm:"uniqueIndex;size:128;not null"`
	MountPath       string `gorm:"uniqueIndex;size:512;not null"`
	RootPath        string `gorm:"size:512;not null"`
	SortOrder       int    `gorm:"not null;default:0"`
	ConfigJSON      string `gorm:"type:text;not null"`
	LastCheckedAt   *time.Time
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
}

// UploadSessionModel 表示上传会话表。
type UploadSessionModel struct {
	UploadID                string    `gorm:"primaryKey;size:64"`
	UserID                  uint      `gorm:"index;not null"`
	SourceID                uint      `gorm:"index;not null"`
	Path                    string    `gorm:"size:1024;not null"`
	TargetVirtualParentPath string    `gorm:"size:1024;not null;default:''"`
	ResolvedSourceID        uint      `gorm:"index;not null;default:0"`
	ResolvedInnerParentPath string    `gorm:"size:1024;not null;default:''"`
	Filename                string    `gorm:"size:255;not null"`
	FileSize                int64     `gorm:"not null"`
	FileHash                string    `gorm:"size:64;not null"`
	ChunkSize               int64     `gorm:"not null"`
	TotalChunks             int       `gorm:"not null"`
	UploadedChunksJSON      string    `gorm:"type:text;not null"`
	StorageDataJSON         string    `gorm:"type:text;not null;default:''"`
	Status                  string    `gorm:"size:32;not null"`
	IsFastUpload            bool      `gorm:"not null;default:false"`
	ExpiresAt               time.Time `gorm:"index;not null"`
	CreatedAt               time.Time `gorm:"not null"`
	UpdatedAt               time.Time `gorm:"not null"`
}

// DownloadTaskModel 表示下载任务表。
type DownloadTaskModel struct {
	ID                      uint    `gorm:"primaryKey"`
	UserID                  uint    `gorm:"index;not null;default:0"`
	Type                    string  `gorm:"size:32;not null"`
	Status                  string  `gorm:"size:32;not null"`
	SourceID                uint    `gorm:"index;not null"`
	SavePath                string  `gorm:"size:1024;not null"`
	TargetVirtualParentPath string  `gorm:"size:1024;not null;default:''"`
	SaveVirtualPath         string  `gorm:"size:1024;not null;default:''"`
	ResolvedSourceID        uint    `gorm:"index;not null;default:0"`
	ResolvedInnerSavePath   string  `gorm:"size:1024;not null;default:''"`
	StagingDir              string  `gorm:"size:1024;not null;default:''"`
	DisplayName             string  `gorm:"size:255;not null"`
	SourceURL               string  `gorm:"type:text;not null"`
	ExternalID              string  `gorm:"size:128"`
	Progress                float64 `gorm:"not null;default:0"`
	DownloadedBytes         int64   `gorm:"not null;default:0"`
	TotalBytes              *int64
	SpeedBytes              int64 `gorm:"not null;default:0"`
	ETASeconds              *int64
	ErrorMessage            *string `gorm:"type:text"`
	FinishedAt              *time.Time
	CreatedAt               time.Time `gorm:"not null"`
	UpdatedAt               time.Time `gorm:"not null"`
}

// TrashItemModel 表示回收站元数据表。
type TrashItemModel struct {
	ID                  uint      `gorm:"primaryKey"`
	SourceID            uint      `gorm:"index;not null"`
	OriginalPath        string    `gorm:"size:1024;not null"`
	OriginalVirtualPath string    `gorm:"size:1024;not null;default:''"`
	TrashPath           string    `gorm:"size:1024;not null"`
	Name                string    `gorm:"size:255;not null"`
	IsDir               bool      `gorm:"not null;default:false"`
	Size                int64     `gorm:"not null;default:0"`
	DeletedAt           time.Time `gorm:"index;not null"`
	ExpiresAt           time.Time `gorm:"index;not null"`
	CreatedAt           time.Time `gorm:"not null"`
	UpdatedAt           time.Time `gorm:"not null"`
}

// ACLRuleModel 表示 ACL 规则表。
type ACLRuleModel struct {
	ID                uint      `gorm:"primaryKey"`
	SourceID          uint      `gorm:"index;not null"`
	Path              string    `gorm:"size:1024;index;not null"`
	VirtualPath       string    `gorm:"size:1024;index;not null;default:''"`
	SubjectType       string    `gorm:"size:32;not null"`
	SubjectID         uint      `gorm:"index;not null"`
	Effect            string    `gorm:"size:16;not null"`
	Priority          int       `gorm:"not null;default:0"`
	Read              bool      `gorm:"not null;default:false"`
	Write             bool      `gorm:"not null;default:false"`
	Delete            bool      `gorm:"not null;default:false"`
	Share             bool      `gorm:"not null;default:false"`
	InheritToChildren bool      `gorm:"not null;default:true"`
	CreatedAt         time.Time `gorm:"not null"`
	UpdatedAt         time.Time `gorm:"not null"`
}

// ShareLinkModel 表示分享链接表。
type ShareLinkModel struct {
	ID                uint       `gorm:"primaryKey"`
	UserID            uint       `gorm:"index;not null"`
	SourceID          uint       `gorm:"index;not null"`
	Path              string     `gorm:"size:1024;not null"`
	TargetVirtualPath string     `gorm:"size:1024;not null;default:''"`
	ResolvedSourceID  uint       `gorm:"index;not null;default:0"`
	ResolvedInnerPath string     `gorm:"size:1024;not null;default:''"`
	Name              string     `gorm:"size:255;not null"`
	IsDir             bool       `gorm:"not null;default:false"`
	Token             string     `gorm:"uniqueIndex;size:128;not null"`
	PasswordHash      *string    `gorm:"size:255"`
	ExpiresAt         *time.Time `gorm:"index"`
	CreatedAt         time.Time  `gorm:"not null"`
	UpdatedAt         time.Time  `gorm:"not null"`
}

// AuditLogModel 表示审计日志表。
type AuditLogModel struct {
	ID               uint      `gorm:"primaryKey"`
	OccurredAt       time.Time `gorm:"index;not null"`
	RequestID        string    `gorm:"index;size:64;not null"`
	EntryPoint       string    `gorm:"index;size:16;not null"`
	ActorUserID      *uint     `gorm:"index"`
	ActorUsername    string    `gorm:"size:64"`
	ActorRoleKey     string    `gorm:"size:32"`
	ClientIP         string    `gorm:"size:64"`
	UserAgent        string    `gorm:"size:512"`
	Method           string    `gorm:"size:16;not null"`
	Path             string    `gorm:"size:1024;not null"`
	ResourceType     string    `gorm:"index;size:64;not null"`
	Action           string    `gorm:"index;size:64;not null"`
	Result           string    `gorm:"index;size:16;not null"`
	ErrorCode        string    `gorm:"size:64"`
	ResourceID       string    `gorm:"size:64"`
	SourceID         *uint     `gorm:"index"`
	VirtualPath      string    `gorm:"index;size:1024"`
	ResolvedSourceID *uint     `gorm:"index"`
	ResolvedPath     string    `gorm:"size:1024"`
	BeforeJSON       string    `gorm:"type:text;not null;default:''"`
	AfterJSON        string    `gorm:"type:text;not null;default:''"`
	DetailJSON       string    `gorm:"type:text;not null;default:''"`
	CreatedAt        time.Time `gorm:"not null"`
}
