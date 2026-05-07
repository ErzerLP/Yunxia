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
	WebDAVEnabled    bool      `gorm:"not null"`
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
	IsEnabled       bool   `gorm:"not null"`
	IsWebDAVExposed bool   `gorm:"not null;default:false"`
	WebDAVReadOnly  bool   `gorm:"not null"`
	WebDAVSlug      string `gorm:"uniqueIndex;size:128;not null"`
	MountPath       string `gorm:"uniqueIndex;size:512;not null"`
	RootPath        string `gorm:"size:512;not null"`
	SortOrder       int    `gorm:"not null;default:0"`
	ConfigJSON      string `gorm:"type:jsonb;not null"`
	LastCheckedAt   *time.Time
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
}

// VFSNodeModel 表示元数据化 VFS 控制面节点表。
type VFSNodeModel struct {
	ID               uint      `gorm:"primaryKey"`
	ParentID         *uint     `gorm:"index;uniqueIndex:idx_vfs_node_parent_name_active,priority:1,where:is_deleted = false"`
	Name             string    `gorm:"size:255;not null;uniqueIndex:idx_vfs_node_parent_name_active,priority:2,where:is_deleted = false"`
	Path             string    `gorm:"size:1024;index;not null;uniqueIndex:idx_vfs_node_path_active,where:is_deleted = false"`
	Kind             string    `gorm:"index;size:32;not null"`
	MountID          *uint     `gorm:"index"`
	ObjectID         *uint     `gorm:"index"`
	SourceID         *uint     `gorm:"index"`
	ProviderItemID   *string   `gorm:"size:512;index"`
	ProviderParentID *string   `gorm:"size:512;index"`
	Size             int64     `gorm:"not null;default:0"`
	MimeType         string    `gorm:"size:255;not null;default:''"`
	ETag             string    `gorm:"column:etag;size:255;not null;default:''"`
	Checksum         string    `gorm:"size:255;not null;default:''"`
	SyncState        string    `gorm:"index;size:32;not null"`
	IsDeleted        bool      `gorm:"index;not null;default:false"`
	CreatedBy        *uint     `gorm:"index"`
	UpdatedBy        *uint     `gorm:"index"`
	CreatedAt        time.Time `gorm:"not null"`
	UpdatedAt        time.Time `gorm:"not null"`
	IndexedAt        *time.Time
	LastSeenAt       *time.Time
}

// StorageObjectModel 表示数据面对象引用表。
type StorageObjectModel struct {
	ID          uint      `gorm:"primaryKey"`
	SourceID    uint      `gorm:"index;not null;uniqueIndex:idx_storage_object_locator,priority:1"`
	DriverType  string    `gorm:"index;size:32;not null;uniqueIndex:idx_storage_object_locator,priority:2"`
	LocatorType string    `gorm:"index;size:64;not null;uniqueIndex:idx_storage_object_locator,priority:3"`
	LocatorJSON string    `gorm:"type:jsonb;not null;uniqueIndex:idx_storage_object_locator,priority:4"`
	Size        int64     `gorm:"not null;default:0"`
	ETag        string    `gorm:"column:etag;size:255;not null;default:''"`
	Checksum    string    `gorm:"size:255;not null;default:''"`
	MimeType    string    `gorm:"size:255;not null;default:''"`
	Status      string    `gorm:"index;size:32;not null"`
	CreatedAt   time.Time `gorm:"not null"`
	UpdatedAt   time.Time `gorm:"not null"`
}

// VFSMountModel 表示 VFS 挂载点元数据表。
type VFSMountModel struct {
	ID              uint      `gorm:"primaryKey"`
	SourceID        uint      `gorm:"index;not null"`
	NodeID          uint      `gorm:"uniqueIndex;not null"`
	MountPath       string    `gorm:"uniqueIndex;size:1024;not null"`
	RootLocatorJSON string    `gorm:"type:jsonb;not null"`
	Mode            string    `gorm:"index;size:32;not null"`
	IsEnabled       bool      `gorm:"index;not null"`
	SortOrder       int       `gorm:"not null;default:0"`
	CreatedAt       time.Time `gorm:"not null"`
	UpdatedAt       time.Time `gorm:"not null"`
}

// VFSTagModel 表示 VFS 标签表。
type VFSTagModel struct {
	ID          uint      `gorm:"primaryKey"`
	OwnerUserID uint      `gorm:"index;not null;default:0;uniqueIndex:idx_vfs_tag_owner_name,priority:1"`
	Name        string    `gorm:"size:128;not null;uniqueIndex:idx_vfs_tag_owner_name,priority:2"`
	Color       string    `gorm:"size:32;not null;default:''"`
	CreatedAt   time.Time `gorm:"not null"`
	UpdatedAt   time.Time `gorm:"not null"`
}

// VFSNodeTagModel 表示 VFS node 与 tag 的绑定表。
type VFSNodeTagModel struct {
	NodeID    uint `gorm:"primaryKey;autoIncrement:false;index"`
	TagID     uint `gorm:"primaryKey;autoIncrement:false;index"`
	CreatedBy *uint
	CreatedAt time.Time `gorm:"not null"`
}

// UploadSessionModel 表示上传会话表。
type UploadSessionModel struct {
	UploadID                string    `gorm:"primaryKey;size:64"`
	UserID                  uint      `gorm:"index;not null"`
	SourceID                uint      `gorm:"index;not null"`
	Path                    string    `gorm:"size:1024;not null"`
	TargetVFSParentNodeID   *uint     `gorm:"index"`
	TargetVirtualParentPath string    `gorm:"size:1024;not null;default:''"`
	ResolvedSourceID        *uint     `gorm:"index"`
	ResolvedInnerParentPath string    `gorm:"size:1024;not null;default:''"`
	ResultVFSNodeID         *uint     `gorm:"index"`
	Filename                string    `gorm:"size:255;not null"`
	FileSize                int64     `gorm:"not null"`
	FileHash                string    `gorm:"size:64;not null"`
	ChunkSize               int64     `gorm:"not null"`
	TotalChunks             int       `gorm:"not null"`
	UploadedChunksJSON      string    `gorm:"type:jsonb;not null"`
	StorageDataJSON         string    `gorm:"type:jsonb;not null"`
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
	DownloaderType          string  `gorm:"size:32;not null;default:'aria2'"`
	Status                  string  `gorm:"size:32;not null"`
	SourceID                uint    `gorm:"index;not null"`
	SavePath                string  `gorm:"size:1024;not null"`
	TargetVFSParentNodeID   *uint   `gorm:"index"`
	TargetVirtualParentPath string  `gorm:"size:1024;not null;default:''"`
	TargetFilename          string  `gorm:"size:255;not null;default:''"`
	SaveVirtualPath         string  `gorm:"size:1024;not null;default:''"`
	ResolvedSourceID        *uint   `gorm:"index"`
	ResolvedInnerSavePath   string  `gorm:"size:1024;not null;default:''"`
	ResultVFSNodeID         *uint   `gorm:"index"`
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

// RSSSourceModel 表示 RSS 源表。
type RSSSourceModel struct {
	ID                     uint   `gorm:"primaryKey"`
	UserID                 uint   `gorm:"index;not null;default:0"`
	Name                   string `gorm:"size:128;not null"`
	URL                    string `gorm:"type:text;not null"`
	IsEnabled              bool   `gorm:"not null"`
	RefreshIntervalSeconds int    `gorm:"not null;default:1800"`
	LastRefreshedAt        *time.Time
	LastError              *string `gorm:"type:text"`
	HealthStatus           string  `gorm:"index;size:32;not null;default:'ok'"`
	ConsecutiveFailures    int     `gorm:"not null;default:0"`
	LastSuccessAt          *time.Time
	NextRefreshAt          *time.Time
	LastRefreshStatus      string    `gorm:"size:32;not null;default:''"`
	LastRefreshStatsJSON   string    `gorm:"type:jsonb;not null"`
	CreatedAt              time.Time `gorm:"not null"`
	UpdatedAt              time.Time `gorm:"not null"`
}

// RSSSubscriptionModel 表示 RSS 订阅规则表。
type RSSSubscriptionModel struct {
	ID                      uint      `gorm:"primaryKey"`
	UserID                  uint      `gorm:"index;not null;default:0"`
	SourceID                uint      `gorm:"index;not null"`
	Name                    string    `gorm:"size:128;not null"`
	IsEnabled               bool      `gorm:"not null"`
	MustContainJSON         string    `gorm:"type:jsonb;not null"`
	MustNotContainJSON      string    `gorm:"type:jsonb;not null"`
	UseRegex                bool      `gorm:"not null;default:false"`
	CaseSensitive           bool      `gorm:"not null;default:false"`
	TargetVFSParentNodeID   *uint     `gorm:"index"`
	TargetVirtualParentPath string    `gorm:"size:1024;not null"`
	DirectoryTemplate       string    `gorm:"size:1024;not null;default:''"`
	FilenameTemplate        string    `gorm:"size:1024;not null;default:''"`
	ResolvedSourceID        *uint     `gorm:"index"`
	ResolvedInnerParentPath string    `gorm:"size:1024;not null;default:''"`
	CreatedAt               time.Time `gorm:"not null"`
	UpdatedAt               time.Time `gorm:"not null"`
}

// RSSItemModel 表示 RSS 条目表。
type RSSItemModel struct {
	ID                    uint   `gorm:"primaryKey"`
	UserID                uint   `gorm:"index;not null;default:0"`
	SourceID              uint   `gorm:"uniqueIndex:idx_rss_item_source_dedup;index;not null"`
	Title                 string `gorm:"type:text;not null"`
	Link                  string `gorm:"type:text;not null"`
	PublishedAt           *time.Time
	GUID                  string  `gorm:"type:text"`
	DedupKey              string  `gorm:"uniqueIndex:idx_rss_item_source_dedup;size:128;not null"`
	DownloadURL           string  `gorm:"type:text;not null;default:''"`
	LinkType              string  `gorm:"size:32;not null;default:'unsupported'"`
	ParsedJSON            string  `gorm:"type:jsonb;not null"`
	Status                string  `gorm:"index;size:32;not null"`
	MatchedSubscriptionID *uint   `gorm:"index"`
	TaskID                *uint   `gorm:"index"`
	ResultVFSNodeID       *uint   `gorm:"index"`
	ErrorMessage          *string `gorm:"type:text"`
	RetryCount            int     `gorm:"not null;default:0"`
	MaxRetryCount         int     `gorm:"not null;default:3"`
	LastAttemptAt         *time.Time
	NextRetryAt           *time.Time `gorm:"index"`
	RetryReason           *string    `gorm:"type:text"`
	CreatedAt             time.Time  `gorm:"not null"`
	UpdatedAt             time.Time  `gorm:"not null"`
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
	InheritToChildren bool      `gorm:"not null"`
	CreatedAt         time.Time `gorm:"not null"`
	UpdatedAt         time.Time `gorm:"not null"`
}

// ShareLinkModel 表示分享链接表。
type ShareLinkModel struct {
	ID                uint       `gorm:"primaryKey"`
	UserID            uint       `gorm:"index;not null"`
	SourceID          uint       `gorm:"index;not null"`
	Path              string     `gorm:"size:1024;not null"`
	TargetVFSNodeID   *uint      `gorm:"index"`
	TargetVirtualPath string     `gorm:"size:1024;not null;default:''"`
	ResolvedSourceID  *uint      `gorm:"index"`
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
	BeforeJSON       string    `gorm:"type:jsonb;not null"`
	AfterJSON        string    `gorm:"type:jsonb;not null"`
	DetailJSON       string    `gorm:"type:jsonb;not null"`
	CreatedAt        time.Time `gorm:"not null"`
}

// NotificationChannelModel 表示通知通道表。
type NotificationChannelModel struct {
	ID             uint      `gorm:"primaryKey"`
	Name           string    `gorm:"size:128;not null"`
	Type           string    `gorm:"index;size:32;not null"`
	IsEnabled      bool      `gorm:"index;not null"`
	EventTypesJSON string    `gorm:"type:jsonb;not null"`
	ConfigJSON     string    `gorm:"type:jsonb;not null"`
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

// NotificationEventModel 表示通知事件表。
type NotificationEventModel struct {
	ID            uint       `gorm:"primaryKey"`
	UserID        uint       `gorm:"index;not null;default:0"`
	EventType     string     `gorm:"index;size:96;not null"`
	Severity      string     `gorm:"size:16;not null"`
	Title         string     `gorm:"size:255;not null"`
	Message       string     `gorm:"type:text;not null"`
	PayloadJSON   string     `gorm:"type:jsonb;not null"`
	Status        string     `gorm:"index;size:32;not null"`
	Attempts      int        `gorm:"not null;default:0"`
	MaxAttempts   int        `gorm:"not null;default:3"`
	LastAttemptAt *time.Time `gorm:"index"`
	NextAttemptAt *time.Time `gorm:"index"`
	DeliveredAt   *time.Time `gorm:"index"`
	LastError     *string    `gorm:"type:text"`
	CreatedAt     time.Time  `gorm:"index;not null"`
	UpdatedAt     time.Time  `gorm:"not null"`
}
