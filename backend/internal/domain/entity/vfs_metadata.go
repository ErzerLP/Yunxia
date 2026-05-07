package entity

import "time"

const (
	VFSNodeKindRoot       = "root"
	VFSNodeKindVirtualDir = "virtual_dir"
	VFSNodeKindMount      = "mount"
	VFSNodeKindDir        = "dir"
	VFSNodeKindFile       = "file"
)

const (
	VFSNodeSyncStateIndexed  = "indexed"
	VFSNodeSyncStatePending  = "pending"
	VFSNodeSyncStateSyncing  = "syncing"
	VFSNodeSyncStateStale    = "stale"
	VFSNodeSyncStateMissing  = "missing"
	VFSNodeSyncStateConflict = "conflict"
	VFSNodeSyncStateError    = "error"
)

const (
	StorageObjectStatusAvailable = "available"
	StorageObjectStatusPending   = "pending"
	StorageObjectStatusUploading = "uploading"
	StorageObjectStatusImporting = "importing"
	StorageObjectStatusMissing   = "missing"
	StorageObjectStatusDeleted   = "deleted"
	StorageObjectStatusError     = "error"
)

const (
	VFSMountModeMirror       = "mirror"
	VFSMountModeImported     = "imported"
	VFSMountModeWriteThrough = "write_through"
)

// VFSNode 表示 Yunxia 虚拟目录树中的控制面节点。
type VFSNode struct {
	ID               uint
	ParentID         *uint
	Name             string
	Path             string
	Kind             string
	MountID          *uint
	ObjectID         *uint
	SourceID         *uint
	ProviderItemID   *string
	ProviderParentID *string
	Size             int64
	MimeType         string
	ETag             string
	Checksum         string
	SyncState        string
	IsDeleted        bool
	CreatedBy        *uint
	UpdatedBy        *uint
	CreatedAt        time.Time
	UpdatedAt        time.Time
	IndexedAt        *time.Time
	LastSeenAt       *time.Time
}

// StorageObject 表示文件节点引用的数据面对象。
type StorageObject struct {
	ID          uint
	SourceID    uint
	DriverType  string
	LocatorType string
	LocatorJSON string
	Size        int64
	ETag        string
	Checksum    string
	MimeType    string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// VFSMount 表示 VFS 控制面中的挂载点。
type VFSMount struct {
	ID              uint
	SourceID        uint
	NodeID          uint
	MountPath       string
	RootLocatorJSON string
	Mode            string
	IsEnabled       bool
	SortOrder       int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// VFSTag 表示绑定到 VFS node 的控制面标签。
type VFSTag struct {
	ID          uint
	OwnerUserID uint
	Name        string
	Color       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// VFSNodeTag 表示 VFS node 与标签的绑定关系。
type VFSNodeTag struct {
	NodeID    uint
	TagID     uint
	CreatedBy *uint
	CreatedAt time.Time
}
