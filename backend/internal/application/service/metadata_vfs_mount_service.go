package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

// MetadataSourceMountSyncer 是 Source/Setup 服务维护 metadata VFS mount 的应用层端口。
type MetadataSourceMountSyncer interface {
	SyncSourceMount(ctx context.Context, source *entity.StorageSource) (*MetadataVFSMountSyncResult, error)
	DisableSourceMount(ctx context.Context, sourceID uint) error
}

// MetadataVFSMountSyncResult 表示单个 source 挂载同步结果。
type MetadataVFSMountSyncResult struct {
	SourceID         uint
	MountPath        string
	Node             *entity.VFSNode
	Mount            *entity.VFSMount
	DisabledMountIDs []uint
}

// MetadataVFSMountSyncAllError 记录批量同步中单个 source 的失败。
type MetadataVFSMountSyncAllError struct {
	SourceID  uint
	MountPath string
	Error     string
}

// MetadataVFSMountSyncAllResult 表示所有 source 挂载同步的聚合结果。
type MetadataVFSMountSyncAllResult struct {
	Synced int
	Failed int
	Errors []MetadataVFSMountSyncAllError
}

// MetadataVFSMountServiceOption 定义 MetadataVFSMountService 可选依赖。
type MetadataVFSMountServiceOption func(*MetadataVFSMountService)

// MetadataVFSMountService 负责把 storage source mount 投影为 VFS 控制面元数据。
type MetadataVFSMountService struct {
	nodeRepo   domainrepo.VFSNodeRepository
	mountRepo  domainrepo.VFSMountRepository
	sourceRepo domainrepo.SourceRepository
	transactor domainrepo.Transactor
	now        func() time.Time
}

// WithMetadataVFSMountTransactor 注册事务端口，保证 source mount 控制面批次一致提交。
func WithMetadataVFSMountTransactor(transactor domainrepo.Transactor) MetadataVFSMountServiceOption {
	return func(s *MetadataVFSMountService) {
		s.transactor = transactor
	}
}

// WithMetadataVFSMountClock 覆盖当前时间，主要用于测试。
func WithMetadataVFSMountClock(now func() time.Time) MetadataVFSMountServiceOption {
	return func(s *MetadataVFSMountService) {
		if now != nil {
			s.now = now
		}
	}
}

// NewMetadataVFSMountService 创建 source mount 元数据同步服务。
func NewMetadataVFSMountService(
	nodeRepo domainrepo.VFSNodeRepository,
	mountRepo domainrepo.VFSMountRepository,
	sourceRepo domainrepo.SourceRepository,
	options ...MetadataVFSMountServiceOption,
) *MetadataVFSMountService {
	service := &MetadataVFSMountService{
		nodeRepo:   nodeRepo,
		mountRepo:  mountRepo,
		sourceRepo: sourceRepo,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// SyncSourceMount 确保 source 的 mount path 在 metadata VFS 中有父级 virtual_dir、mount node 与 vfs_mount row。
func (s *MetadataVFSMountService) SyncSourceMount(ctx context.Context, source *entity.StorageSource) (*MetadataVFSMountSyncResult, error) {
	if s == nil || s.nodeRepo == nil || s.mountRepo == nil {
		return nil, ErrSourceDriverUnsupported
	}
	if source == nil || source.ID == 0 {
		return nil, ErrPathInvalid
	}
	mountPath, err := normalizeMountPath(source.MountPath)
	if err != nil {
		return nil, err
	}
	rootPath, err := normalizeVirtualPath(source.RootPath)
	if err != nil {
		return nil, err
	}

	result := &MetadataVFSMountSyncResult{
		SourceID:  source.ID,
		MountPath: mountPath,
	}
	err = s.withinMountTx(ctx, func(txCtx context.Context) error {
		now := s.now()
		if err := s.ensureMountPathReusable(txCtx, source.ID, mountPath); err != nil {
			return err
		}
		mountNode, err := s.ensureSourceMountNode(txCtx, source, mountPath, now)
		if err != nil {
			return err
		}
		rootLocatorJSON, err := metadataVFSMountRootLocatorJSON(source, mountPath, rootPath)
		if err != nil {
			return err
		}

		mount := &entity.VFSMount{
			SourceID:        source.ID,
			NodeID:          mountNode.ID,
			MountPath:       mountPath,
			RootLocatorJSON: rootLocatorJSON,
			Mode:            entity.VFSMountModeMirror,
			IsEnabled:       source.IsEnabled,
			SortOrder:       source.SortOrder,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := s.mountRepo.UpsertByMountPath(txCtx, mount); err != nil {
			return normalizeMetadataVFSError(err)
		}
		if !sameUintPtr(mountNode.MountID, &mount.ID) || mountNode.SyncState != entity.VFSNodeSyncStateIndexed {
			mountNode.MountID = &mount.ID
			mountNode.SyncState = entity.VFSNodeSyncStateIndexed
			mountNode.UpdatedAt = now
			if mountNode.IndexedAt == nil {
				mountNode.IndexedAt = &now
			}
			if err := s.nodeRepo.Update(txCtx, mountNode); err != nil {
				return normalizeMetadataVFSError(err)
			}
		}
		disabled, err := s.disableOtherSourceMounts(txCtx, source.ID, mountPath, now)
		if err != nil {
			return err
		}

		result.Node = mountNode
		result.Mount = mount
		result.DisabledMountIDs = disabled
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// SyncAllSourceMounts 同步所有 source 的 mount 控制面；单个 source 失败会记录并继续。
func (s *MetadataVFSMountService) SyncAllSourceMounts(ctx context.Context) (*MetadataVFSMountSyncAllResult, error) {
	if s == nil || s.sourceRepo == nil {
		return nil, ErrSourceDriverUnsupported
	}
	sources, err := s.sourceRepo.ListAll(ctx)
	if err != nil {
		return nil, err
	}

	result := &MetadataVFSMountSyncAllResult{}
	for _, source := range sources {
		if _, syncErr := s.SyncSourceMount(ctx, source); syncErr != nil {
			result.Failed++
			item := MetadataVFSMountSyncAllError{Error: syncErr.Error()}
			if source != nil {
				item.SourceID = source.ID
				item.MountPath = source.MountPath
			}
			result.Errors = append(result.Errors, item)
			continue
		}
		result.Synced++
	}
	return result, nil
}

// DisableSourceMount 禁用指定 source 的所有 VFS mount，保留 mount node 与子树。
func (s *MetadataVFSMountService) DisableSourceMount(ctx context.Context, sourceID uint) error {
	if s == nil || s.mountRepo == nil {
		return ErrSourceDriverUnsupported
	}
	if sourceID == 0 {
		return ErrPathInvalid
	}
	return s.withinMountTx(ctx, func(txCtx context.Context) error {
		_, err := s.disableOtherSourceMounts(txCtx, sourceID, "", s.now())
		return err
	})
}

func (s *MetadataVFSMountService) ensureSourceMountNode(
	ctx context.Context,
	source *entity.StorageSource,
	mountPath string,
	now time.Time,
) (*entity.VFSNode, error) {
	root, err := s.ensureMountRoot(ctx, now)
	if err != nil {
		return nil, err
	}
	if mountPath == "/" {
		root.SourceID = &source.ID
		root.SyncState = entity.VFSNodeSyncStateIndexed
		root.UpdatedAt = now
		if root.IndexedAt == nil {
			root.IndexedAt = &now
		}
		if err := s.nodeRepo.Update(ctx, root); err != nil {
			return nil, normalizeMetadataVFSError(err)
		}
		return root, nil
	}

	current := root
	segments := strings.Split(strings.TrimPrefix(mountPath, "/"), "/")
	for index, segment := range segments {
		if segment == "" {
			continue
		}
		currentPath := joinVirtualPath(current.Path, segment)
		isMountNode := index == len(segments)-1
		desiredKind := entity.VFSNodeKindVirtualDir
		desiredSourceID := (*uint)(nil)
		if isMountNode {
			desiredKind = entity.VFSNodeKindMount
			desiredSourceID = &source.ID
		}

		existing, err := s.nodeRepo.FindByPath(ctx, currentPath)
		if err == nil {
			if !metadataVFSNodeIsDirectory(existing) {
				return nil, ErrNameConflict
			}
			if existing.Kind == entity.VFSNodeKindMount &&
				existing.SourceID != nil &&
				desiredSourceID != nil &&
				*existing.SourceID != *desiredSourceID {
				reusable, err := s.mountNodeReusableForSource(ctx, existing.ID, *desiredSourceID)
				if err != nil {
					return nil, err
				}
				if !reusable {
					return nil, ErrSourceMountPathConflict
				}
			}
			if metadataVFSMountNodeNeedsUpdate(existing, current.ID, segment, desiredKind, desiredSourceID) {
				existing.ParentID = &current.ID
				existing.Name = segment
				existing.Kind = desiredKind
				existing.SourceID = cloneUintPtr(desiredSourceID)
				existing.SyncState = entity.VFSNodeSyncStateIndexed
				existing.MimeType = "inode/directory"
				existing.UpdatedAt = now
				if existing.IndexedAt == nil {
					existing.IndexedAt = &now
				}
				if err := s.nodeRepo.Update(ctx, existing); err != nil {
					return nil, normalizeMetadataVFSError(err)
				}
			}
			current = existing
			continue
		}
		if !errors.Is(err, domainrepo.ErrNotFound) {
			return nil, normalizeMetadataVFSError(err)
		}

		node := &entity.VFSNode{
			ParentID:   &current.ID,
			Name:       segment,
			Path:       currentPath,
			Kind:       desiredKind,
			SourceID:   cloneUintPtr(desiredSourceID),
			MimeType:   "inode/directory",
			SyncState:  entity.VFSNodeSyncStateIndexed,
			CreatedAt:  now,
			UpdatedAt:  now,
			IndexedAt:  &now,
			LastSeenAt: &now,
		}
		if err := s.nodeRepo.Create(ctx, node); err != nil {
			return nil, normalizeMetadataVFSError(err)
		}
		current = node
	}
	return current, nil
}

func (s *MetadataVFSMountService) ensureMountRoot(ctx context.Context, now time.Time) (*entity.VFSNode, error) {
	root := &entity.VFSNode{
		Name:      "",
		Path:      "/",
		Kind:      entity.VFSNodeKindRoot,
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedAt: now,
		UpdatedAt: now,
		IndexedAt: &now,
	}
	if err := s.nodeRepo.UpsertByPath(ctx, root); err != nil {
		return nil, normalizeMetadataVFSError(err)
	}
	return root, nil
}

func (s *MetadataVFSMountService) ensureMountPathReusable(ctx context.Context, sourceID uint, mountPath string) error {
	existing, err := s.mountRepo.FindByMountPath(ctx, mountPath)
	if err != nil {
		if errors.Is(err, domainrepo.ErrNotFound) {
			return nil
		}
		return normalizeMetadataVFSError(err)
	}
	if existing.SourceID != sourceID && existing.IsEnabled {
		return ErrSourceMountPathConflict
	}
	return nil
}

func (s *MetadataVFSMountService) mountNodeReusableForSource(ctx context.Context, nodeID uint, sourceID uint) (bool, error) {
	if s.mountRepo == nil || nodeID == 0 {
		return true, nil
	}
	mount, err := s.mountRepo.FindByNodeID(ctx, nodeID)
	if err != nil {
		if errors.Is(err, domainrepo.ErrNotFound) {
			return true, nil
		}
		return false, normalizeMetadataVFSError(err)
	}
	return mount.SourceID == sourceID || !mount.IsEnabled, nil
}

func (s *MetadataVFSMountService) disableOtherSourceMounts(ctx context.Context, sourceID uint, keepMountPath string, now time.Time) ([]uint, error) {
	mounts, err := s.mountRepo.List(ctx, domainrepo.VFSMountListFilter{
		SourceID:      sourceID,
		IncludeHidden: true,
	})
	if err != nil {
		return nil, normalizeMetadataVFSError(err)
	}

	disabled := make([]uint, 0)
	for _, mount := range mounts {
		if mount == nil || mount.MountPath == keepMountPath {
			continue
		}
		if mount.IsEnabled {
			mount.IsEnabled = false
			mount.UpdatedAt = now
			if err := s.mountRepo.Update(ctx, mount); err != nil {
				return nil, normalizeMetadataVFSError(err)
			}
			disabled = append(disabled, mount.ID)
		}
		if err := s.markMountNodeStale(ctx, mount.NodeID, now); err != nil {
			return nil, err
		}
	}
	return disabled, nil
}

func (s *MetadataVFSMountService) markMountNodeStale(ctx context.Context, nodeID uint, now time.Time) error {
	if s.nodeRepo == nil || nodeID == 0 {
		return nil
	}
	node, err := s.nodeRepo.FindByID(ctx, nodeID)
	if err != nil {
		if errors.Is(err, domainrepo.ErrNotFound) {
			return nil
		}
		return normalizeMetadataVFSError(err)
	}
	if node.IsDeleted {
		return nil
	}
	node.SyncState = entity.VFSNodeSyncStateStale
	node.UpdatedAt = now
	return normalizeMetadataVFSError(s.nodeRepo.Update(ctx, node))
}

func (s *MetadataVFSMountService) withinMountTx(ctx context.Context, fn func(context.Context) error) error {
	if s.transactor == nil {
		return fn(ctx)
	}
	return s.transactor.WithinTx(ctx, fn)
}

func metadataVFSMountNodeNeedsUpdate(existing *entity.VFSNode, parentID uint, name string, kind string, sourceID *uint) bool {
	if existing == nil {
		return false
	}
	return existing.ParentID == nil ||
		*existing.ParentID != parentID ||
		existing.Name != name ||
		existing.Kind != kind ||
		!sameUintPtr(existing.SourceID, sourceID) ||
		existing.SyncState != entity.VFSNodeSyncStateIndexed
}

func metadataVFSMountRootLocatorJSON(source *entity.StorageSource, mountPath string, rootPath string) (string, error) {
	payload := map[string]any{
		"source_id":        source.ID,
		"driver_type":      source.DriverType,
		"mount_path":       mountPath,
		"source_root_path": rootPath,
	}
	if configRoot := metadataVFSMountConfigRootSnapshot(source.ConfigJSON); len(configRoot) > 0 {
		payload["config_root"] = configRoot
	}
	return metadataCommitMarshalLocatorJSON(payload)
}

func metadataVFSMountConfigRootSnapshot(rawConfigJSON string) map[string]any {
	var config map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(rawConfigJSON)), &config); err != nil {
		return nil
	}
	snapshot := make(map[string]any)
	for key, value := range config {
		if metadataCommitLocatorKeyIsSensitive(key) {
			continue
		}
		if !metadataVFSMountConfigRootKey(key) {
			continue
		}
		if metadataCommitLocatorKeyIsPhysical(key) {
			snapshot[key] = "[redacted]"
			continue
		}
		snapshot[key] = metadataCommitSanitizeLocatorValue(value)
	}
	return snapshot
}

func metadataVFSMountConfigRootKey(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("-", "_", " ", "_").Replace(strings.TrimSpace(key)))
	switch normalized {
	case "root", "root_path", "root_id", "root_folder_id", "root_folder_name",
		"folder_id", "parent_id", "bucket", "base_prefix", "prefix", "base_path":
		return true
	default:
		return strings.Contains(normalized, "root") || strings.HasSuffix(normalized, "_prefix")
	}
}
