package service

import "yunxia/internal/domain/entity"

// VirtualEntryKind 表示虚拟文件系统中的节点类型。
type VirtualEntryKind string

const (
	// VirtualEntryKindFile 表示文件节点。
	VirtualEntryKindFile VirtualEntryKind = "file"
	// VirtualEntryKindDirectory 表示目录节点。
	VirtualEntryKindDirectory VirtualEntryKind = "directory"
)

// MountEntry 表示运行时挂载信息。
type MountEntry struct {
	MountPath string
	Source    *entity.StorageSource
}

// ResolvedPath 表示虚拟路径解析结果。
type ResolvedPath struct {
	VirtualPath      string
	MatchedMountPath string
	InnerPath        string
	Source           *entity.StorageSource
	IsRealMount      bool
	IsPureVirtual    bool
}
