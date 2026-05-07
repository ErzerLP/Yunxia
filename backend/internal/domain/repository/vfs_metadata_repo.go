package repository

import (
	"context"

	"yunxia/internal/domain/entity"
)

// VFSNodeListFilter 定义 VFS node 列表筛选条件。
type VFSNodeListFilter struct {
	Kind           string
	MountID        *uint
	SourceID       *uint
	SyncState      string
	IncludeDeleted bool
	Limit          int
}

// StorageObjectListFilter 定义 storage object 列表筛选条件。
type StorageObjectListFilter struct {
	SourceID    uint
	DriverType  string
	LocatorType string
	Status      string
	Limit       int
}

// VFSMountListFilter 定义 VFS mount 列表筛选条件。
type VFSMountListFilter struct {
	SourceID      uint
	Enabled       *bool
	Mode          string
	PathPrefix    string
	IncludeHidden bool
}

// VFSTagListFilter 定义 VFS tag 列表筛选条件。
type VFSTagListFilter struct {
	OwnerUserID uint
	Name        string
	Limit       int
}

// VFSNodeRepository 定义 VFS 控制面节点仓储能力。
type VFSNodeRepository interface {
	Create(ctx context.Context, node *entity.VFSNode) error
	Update(ctx context.Context, node *entity.VFSNode) error
	// Delete 软删除指定节点及其 path 子树，保留元数据用于回收站/审计/恢复。
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*entity.VFSNode, error)
	FindByPath(ctx context.Context, path string) (*entity.VFSNode, error)
	ListChildren(ctx context.Context, parentID *uint, filter VFSNodeListFilter) ([]*entity.VFSNode, error)
	ListByPathPrefix(ctx context.Context, pathPrefix string, filter VFSNodeListFilter) ([]*entity.VFSNode, error)
	UpsertByPath(ctx context.Context, node *entity.VFSNode) error
}

// StorageObjectRepository 定义数据面对象仓储能力。
type StorageObjectRepository interface {
	Create(ctx context.Context, object *entity.StorageObject) error
	Update(ctx context.Context, object *entity.StorageObject) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*entity.StorageObject, error)
	List(ctx context.Context, filter StorageObjectListFilter) ([]*entity.StorageObject, error)
}

// VFSMountRepository 定义 VFS 挂载点仓储能力。
type VFSMountRepository interface {
	Create(ctx context.Context, mount *entity.VFSMount) error
	Update(ctx context.Context, mount *entity.VFSMount) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*entity.VFSMount, error)
	FindByNodeID(ctx context.Context, nodeID uint) (*entity.VFSMount, error)
	FindByMountPath(ctx context.Context, mountPath string) (*entity.VFSMount, error)
	List(ctx context.Context, filter VFSMountListFilter) ([]*entity.VFSMount, error)
	UpsertByMountPath(ctx context.Context, mount *entity.VFSMount) error
}

// VFSTagRepository 定义 VFS tag 与 node/tag 绑定仓储能力。
type VFSTagRepository interface {
	Create(ctx context.Context, tag *entity.VFSTag) error
	Update(ctx context.Context, tag *entity.VFSTag) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*entity.VFSTag, error)
	FindByOwnerAndName(ctx context.Context, ownerUserID uint, name string) (*entity.VFSTag, error)
	List(ctx context.Context, filter VFSTagListFilter) ([]*entity.VFSTag, error)
	UpsertByOwnerAndName(ctx context.Context, tag *entity.VFSTag) error
	AttachToNode(ctx context.Context, binding *entity.VFSNodeTag) error
	DetachFromNode(ctx context.Context, nodeID, tagID uint) error
	ListTagsForNode(ctx context.Context, nodeID uint) ([]*entity.VFSTag, error)
	ListBindingsForNode(ctx context.Context, nodeID uint) ([]*entity.VFSNodeTag, error)
	ListNodeIDsByTag(ctx context.Context, tagID uint) ([]uint, error)
}
