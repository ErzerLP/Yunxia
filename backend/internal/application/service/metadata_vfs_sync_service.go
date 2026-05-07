package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

// MetadataVFSRefreshResult 表示一次 metadata VFS 懒索引/同步刷新统计。
type MetadataVFSRefreshResult struct {
	Path      string
	NodeID    uint
	Seen      int
	Indexed   int
	Updated   int
	Missing   int
	Conflicts int
	Errors    int
	SyncState string
	Error     string
}

// MetadataVFSSyncServiceOption 定义 MetadataVFSSyncService 可选依赖。
type MetadataVFSSyncServiceOption func(*MetadataVFSSyncService)

// MetadataVFSSyncService 将 driver/indexer 列表结果同步为 VFS nodes 与 storage objects。
type MetadataVFSSyncService struct {
	nodeRepo   domainrepo.VFSNodeRepository
	objectRepo domainrepo.StorageObjectRepository
	sourceRepo domainrepo.SourceRepository
	mountRepo  domainrepo.VFSMountRepository

	indexers   map[string]RemoteIndexer
	transactor domainrepo.Transactor
	now        func() time.Time
}

// WithMetadataVFSSyncMountRepository 注册 VFS mount 仓储，用于解析 mount root/inner path。
func WithMetadataVFSSyncMountRepository(mountRepo domainrepo.VFSMountRepository) MetadataVFSSyncServiceOption {
	return func(s *MetadataVFSSyncService) {
		s.mountRepo = mountRepo
	}
}

// WithMetadataVFSSyncIndexer 注册指定 driver 的远端索引器。
func WithMetadataVFSSyncIndexer(driverType string, indexer RemoteIndexer) MetadataVFSSyncServiceOption {
	return func(s *MetadataVFSSyncService) {
		if driverType == "" || indexer == nil {
			return
		}
		if s.indexers == nil {
			s.indexers = make(map[string]RemoteIndexer)
		}
		s.indexers[driverType] = indexer
	}
}

// WithMetadataVFSSyncFileDriver 使用现有 FileDriver.List 作为懒索引兼容桥。
func WithMetadataVFSSyncFileDriver(driverType string, driver FileDriver) MetadataVFSSyncServiceOption {
	return WithMetadataVFSSyncIndexer(driverType, fileDriverRemoteIndexer{driver: driver})
}

// WithMetadataVFSSyncTransactor 注册事务端口，保证 node/object 刷新批次一致提交。
func WithMetadataVFSSyncTransactor(transactor domainrepo.Transactor) MetadataVFSSyncServiceOption {
	return func(s *MetadataVFSSyncService) {
		s.transactor = transactor
	}
}

// WithMetadataVFSSyncClock 覆盖当前时间，主要用于测试。
func WithMetadataVFSSyncClock(now func() time.Time) MetadataVFSSyncServiceOption {
	return func(s *MetadataVFSSyncService) {
		if now != nil {
			s.now = now
		}
	}
}

// NewMetadataVFSSyncService 创建 metadata VFS 懒索引/同步服务。
func NewMetadataVFSSyncService(
	nodeRepo domainrepo.VFSNodeRepository,
	objectRepo domainrepo.StorageObjectRepository,
	sourceRepo domainrepo.SourceRepository,
	options ...MetadataVFSSyncServiceOption,
) *MetadataVFSSyncService {
	service := &MetadataVFSSyncService{
		nodeRepo:   nodeRepo,
		objectRepo: objectRepo,
		sourceRepo: sourceRepo,
		indexers: map[string]RemoteIndexer{
			"local": localFilesystemRemoteIndexer{},
		},
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// RefreshPath 按 metadata path 刷新目录直接子项。
func (s *MetadataVFSSyncService) RefreshPath(ctx context.Context, targetPath string) (*MetadataVFSRefreshResult, error) {
	if s == nil || s.nodeRepo == nil {
		return nil, ErrSourceDriverUnsupported
	}
	normalizedPath, err := normalizeVirtualPath(targetPath)
	if err != nil {
		return nil, err
	}
	node, err := s.nodeRepo.FindByPath(ctx, normalizedPath)
	if err != nil {
		return nil, normalizeMetadataVFSError(err)
	}
	return s.RefreshNode(ctx, node)
}

// RefreshNode 刷新指定 metadata 目录/mount 的直接子项。
func (s *MetadataVFSSyncService) RefreshNode(ctx context.Context, target *entity.VFSNode) (*MetadataVFSRefreshResult, error) {
	result := &MetadataVFSRefreshResult{}
	if target != nil {
		result.Path = target.Path
		result.NodeID = target.ID
		result.SyncState = target.SyncState
	}
	if s == nil || s.nodeRepo == nil || s.objectRepo == nil || s.sourceRepo == nil {
		result.Errors = 1
		result.Error = ErrSourceDriverUnsupported.Error()
		return result, ErrSourceDriverUnsupported
	}
	if target == nil || target.IsDeleted {
		result.Errors = 1
		result.Error = ErrFileNotFound.Error()
		return result, ErrFileNotFound
	}
	if !metadataVFSNodeIsDirectory(target) {
		result.Errors = 1
		result.Error = ErrPathInvalid.Error()
		return result, ErrPathInvalid
	}
	if target.SourceID == nil {
		result.Errors = 1
		result.Error = ErrNoBackingStorage.Error()
		return result, ErrNoBackingStorage
	}

	source, err := s.sourceRepo.FindByID(ctx, *target.SourceID)
	if err != nil {
		result.Errors = 1
		result.Error = normalizeMetadataVFSError(err).Error()
		return result, normalizeMetadataVFSError(err)
	}
	indexer, exists := s.indexers[source.DriverType]
	if !exists || indexer == nil {
		stableErr := ErrSourceDriverUnsupported
		if markErr := s.markTargetSyncState(ctx, target, entity.VFSNodeSyncStateError); markErr != nil {
			result.Errors = 1
			result.Error = markErr.Error()
			return result, markErr
		}
		result.Errors = 1
		result.SyncState = entity.VFSNodeSyncStateError
		result.Error = stableErr.Error()
		return result, stableErr
	}

	req, err := s.buildRemoteListRequest(ctx, target, source)
	if err != nil {
		result.Errors = 1
		result.Error = err.Error()
		return result, err
	}

	entries, err := indexer.ListRemoteChildren(ctx, source, req)
	if err != nil {
		stableErr := normalizeMetadataVFSSyncProviderError(err)
		if markErr := s.markTargetSyncState(ctx, target, entity.VFSNodeSyncStateError); markErr != nil {
			result.Errors = 1
			result.Error = markErr.Error()
			return result, markErr
		}
		result.Errors = 1
		result.SyncState = entity.VFSNodeSyncStateError
		result.Error = stableErr.Error()
		return result, stableErr
	}

	if err := s.applyRemoteEntries(ctx, target, source, req, entries, result); err != nil {
		return result, err
	}
	if result.Conflicts > 0 {
		result.Error = ErrVFSSyncConflict.Error()
		return result, ErrVFSSyncConflict
	}
	return result, nil
}

func (s *MetadataVFSSyncService) buildRemoteListRequest(ctx context.Context, target *entity.VFSNode, source *entity.StorageSource) (RemoteListRequest, error) {
	mountPath := ""
	rootLocatorJSON := ""

	if target.Kind == entity.VFSNodeKindMount {
		mountPath = target.Path
	}
	if s.mountRepo != nil {
		mount, err := s.findMountForSyncTarget(ctx, target)
		if err == nil && mount != nil {
			mountPath = mount.MountPath
			rootLocatorJSON = mount.RootLocatorJSON
		} else if err != nil && !errors.Is(err, domainrepo.ErrNotFound) {
			return RemoteListRequest{}, normalizeMetadataVFSError(err)
		}
	}
	if mountPath == "" && source != nil {
		mountPath = source.MountPath
	}

	parentPath := "/"
	if mountPath != "" {
		normalizedMountPath, err := normalizeVirtualPath(mountPath)
		if err != nil {
			return RemoteListRequest{}, err
		}
		if !isSubPath(normalizedMountPath, target.Path) {
			return RemoteListRequest{}, ErrPathInvalid
		}
		parentPath = strings.TrimPrefix(target.Path, normalizedMountPath)
		if parentPath == "" {
			parentPath = "/"
		}
		if !strings.HasPrefix(parentPath, "/") {
			parentPath = "/" + parentPath
		}
	} else if target.Kind != entity.VFSNodeKindMount {
		var err error
		parentPath, err = normalizeVirtualPath(target.Path)
		if err != nil {
			return RemoteListRequest{}, err
		}
	}

	req := RemoteListRequest{
		ParentPath:           parentPath,
		MountRootLocatorJSON: rootLocatorJSON,
	}
	if target.ProviderItemID != nil {
		req.ParentProviderItemID = *target.ProviderItemID
	}
	if target.ProviderItemID != nil || target.ProviderParentID != nil {
		payload := map[string]string{}
		if target.ProviderItemID != nil {
			payload["provider_item_id"] = *target.ProviderItemID
		}
		if target.ProviderParentID != nil {
			payload["provider_parent_id"] = *target.ProviderParentID
		}
		if raw, err := json.Marshal(payload); err == nil {
			req.ParentLocatorJSON = string(raw)
		}
	}
	return req, nil
}

func (s *MetadataVFSSyncService) findMountForSyncTarget(ctx context.Context, target *entity.VFSNode) (*entity.VFSMount, error) {
	if target == nil || s.mountRepo == nil {
		return nil, domainrepo.ErrNotFound
	}
	if target.MountID != nil {
		return s.mountRepo.FindByID(ctx, *target.MountID)
	}
	if target.Kind == entity.VFSNodeKindMount {
		return s.mountRepo.FindByNodeID(ctx, target.ID)
	}
	return nil, domainrepo.ErrNotFound
}

func (s *MetadataVFSSyncService) applyRemoteEntries(
	ctx context.Context,
	target *entity.VFSNode,
	source *entity.StorageSource,
	req RemoteListRequest,
	entries []RemoteEntry,
	result *MetadataVFSRefreshResult,
) error {
	now := s.now()
	return s.withinSyncTx(ctx, func(txCtx context.Context) error {
		children, err := s.nodeRepo.ListChildren(txCtx, &target.ID, domainrepo.VFSNodeListFilter{})
		if err != nil {
			result.Errors++
			result.Error = normalizeMetadataVFSError(err).Error()
			return normalizeMetadataVFSError(err)
		}

		childrenByName := make(map[string]*entity.VFSNode, len(children))
		for _, child := range children {
			childrenByName[child.Name] = child
		}

		nameCounts := map[string]int{}
		firstEntryByName := map[string]RemoteEntry{}
		for _, entry := range entries {
			name := remoteEntryName(entry)
			if err := validateFileName(name); err != nil {
				result.Conflicts++
				continue
			}
			nameCounts[name]++
			if _, exists := firstEntryByName[name]; !exists {
				firstEntryByName[name] = entry
			}
		}

		seenNames := make(map[string]struct{}, len(nameCounts))
		for name, count := range nameCounts {
			if count <= 1 {
				continue
			}
			seenNames[name] = struct{}{}
			if err := s.upsertConflictChild(txCtx, target, firstEntryByName[name], now); err != nil {
				result.Errors++
				result.Error = err.Error()
				return err
			}
			result.Conflicts++
		}

		for _, entry := range entries {
			name := remoteEntryName(entry)
			if _, duplicated := seenNames[name]; duplicated {
				continue
			}
			if nameCounts[name] == 0 {
				continue
			}
			if err := validateFileName(name); err != nil {
				continue
			}
			seenNames[name] = struct{}{}
			result.Seen++

			existing := childrenByName[name]
			if metadataVFSChildIsControlNode(existing) {
				// 挂载点 / 纯虚拟目录属于 Yunxia 控制面，不被底层同名对象覆盖。
				continue
			}
			if metadataVFSRemoteIdentityConflicts(existing, entry) {
				if err := s.markChildConflict(txCtx, existing, now); err != nil {
					result.Errors++
					result.Error = err.Error()
					return err
				}
				result.Conflicts++
				continue
			}

			child, created, changed, err := s.upsertRemoteChild(txCtx, target, source, req, entry, existing, now)
			if err != nil {
				if errors.Is(err, ErrNameConflict) {
					result.Conflicts++
					if existing != nil {
						if markErr := s.markChildConflict(txCtx, existing, now); markErr != nil {
							result.Errors++
							result.Error = markErr.Error()
							return markErr
						}
					}
					continue
				}
				result.Errors++
				result.Error = err.Error()
				return err
			}
			childrenByName[name] = child
			if created {
				result.Indexed++
			} else if changed {
				result.Updated++
			}
		}

		for _, child := range children {
			if _, seen := seenNames[child.Name]; seen {
				continue
			}
			if metadataVFSChildIsControlNode(child) {
				continue
			}
			child.SyncState = entity.VFSNodeSyncStateMissing
			child.UpdatedAt = now
			if err := s.nodeRepo.Update(txCtx, child); err != nil {
				result.Errors++
				result.Error = normalizeMetadataVFSError(err).Error()
				return normalizeMetadataVFSError(err)
			}
			if child.ObjectID != nil {
				if err := s.markObjectMissing(txCtx, *child.ObjectID, now); err != nil {
					result.Errors++
					result.Error = err.Error()
					return err
				}
			}
			result.Missing++
		}

		state := entity.VFSNodeSyncStateIndexed
		if result.Conflicts > 0 {
			state = entity.VFSNodeSyncStateConflict
		}
		target.SyncState = state
		target.UpdatedAt = now
		target.IndexedAt = &now
		target.LastSeenAt = &now
		if err := s.nodeRepo.Update(txCtx, target); err != nil {
			result.Errors++
			result.Error = normalizeMetadataVFSError(err).Error()
			return normalizeMetadataVFSError(err)
		}
		result.SyncState = state
		return nil
	})
}

func (s *MetadataVFSSyncService) upsertRemoteChild(
	ctx context.Context,
	target *entity.VFSNode,
	source *entity.StorageSource,
	req RemoteListRequest,
	entry RemoteEntry,
	existing *entity.VFSNode,
	now time.Time,
) (*entity.VFSNode, bool, bool, error) {
	name := remoteEntryName(entry)
	entryPath := normalizeRemoteEntryPath(req.ParentPath, entry, name)
	childPath := joinVirtualPath(target.Path, name)

	var objectID *uint
	mimeType := remoteEntryMimeType(entry)
	if !entry.IsDir {
		object, err := s.upsertRemoteObject(ctx, source, entry, entryPath, mimeType, now)
		if err != nil {
			return nil, false, false, err
		}
		objectID = &object.ID
	}

	node := &entity.VFSNode{
		ParentID:         &target.ID,
		Name:             name,
		Path:             childPath,
		Kind:             metadataVFSRemoteEntryKind(entry),
		MountID:          cloneUintPtr(target.MountID),
		ObjectID:         objectID,
		SourceID:         cloneUintPtr(target.SourceID),
		ProviderItemID:   stringPtrOrNil(entry.ProviderItemID),
		ProviderParentID: providerParentIDForEntry(target, entry),
		Size:             remoteEntrySize(entry),
		MimeType:         mimeType,
		ETag:             entry.ETag,
		Checksum:         entry.Checksum,
		SyncState:        entity.VFSNodeSyncStateIndexed,
		CreatedAt:        now,
		UpdatedAt:        now,
		IndexedAt:        &now,
		LastSeenAt:       &now,
	}

	created := existing == nil
	changed := created || metadataVFSNodeChangedByRemote(existing, node)
	if existing != nil {
		node.ID = existing.ID
		node.CreatedAt = existing.CreatedAt
		node.CreatedBy = cloneUintPtr(existing.CreatedBy)
	}
	if err := s.nodeRepo.UpsertByPath(ctx, node); err != nil {
		return nil, false, false, normalizeMetadataVFSError(err)
	}
	return node, created, changed, nil
}

func (s *MetadataVFSSyncService) upsertConflictChild(
	ctx context.Context,
	target *entity.VFSNode,
	entry RemoteEntry,
	now time.Time,
) error {
	name := remoteEntryName(entry)
	if err := validateFileName(name); err != nil {
		return nil
	}
	childPath := joinVirtualPath(target.Path, name)
	existing, err := s.nodeRepo.FindByPath(ctx, childPath)
	if err != nil && !errors.Is(err, domainrepo.ErrNotFound) {
		return normalizeMetadataVFSError(err)
	}

	node := &entity.VFSNode{
		ParentID:         &target.ID,
		Name:             name,
		Path:             childPath,
		Kind:             metadataVFSRemoteEntryKind(entry),
		MountID:          cloneUintPtr(target.MountID),
		SourceID:         cloneUintPtr(target.SourceID),
		ProviderItemID:   stringPtrOrNil(entry.ProviderItemID),
		ProviderParentID: providerParentIDForEntry(target, entry),
		Size:             remoteEntrySize(entry),
		MimeType:         remoteEntryMimeType(entry),
		ETag:             entry.ETag,
		Checksum:         entry.Checksum,
		SyncState:        entity.VFSNodeSyncStateConflict,
		CreatedAt:        now,
		UpdatedAt:        now,
		IndexedAt:        &now,
		LastSeenAt:       &now,
	}
	if existing != nil {
		node.ID = existing.ID
		node.CreatedAt = existing.CreatedAt
		node.CreatedBy = cloneUintPtr(existing.CreatedBy)
	}
	if err := s.nodeRepo.UpsertByPath(ctx, node); err != nil {
		return normalizeMetadataVFSError(err)
	}
	return nil
}

func (s *MetadataVFSSyncService) upsertRemoteObject(ctx context.Context, source *entity.StorageSource, entry RemoteEntry, entryPath string, mimeType string, now time.Time) (*entity.StorageObject, error) {
	locatorType, locatorJSON, err := metadataVFSRemoteEntryLocator(source, entry, entryPath)
	if err != nil {
		return nil, err
	}
	object := &entity.StorageObject{
		SourceID:    source.ID,
		DriverType:  source.DriverType,
		LocatorType: locatorType,
		LocatorJSON: locatorJSON,
		Size:        remoteEntrySize(entry),
		ETag:        entry.ETag,
		Checksum:    entry.Checksum,
		MimeType:    mimeType,
		Status:      entity.StorageObjectStatusAvailable,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.objectRepo.UpsertByLocator(ctx, object); err != nil {
		return nil, normalizeMetadataVFSError(err)
	}
	return object, nil
}

func (s *MetadataVFSSyncService) markObjectMissing(ctx context.Context, objectID uint, now time.Time) error {
	object, err := s.objectRepo.FindByID(ctx, objectID)
	if err != nil {
		if errors.Is(err, domainrepo.ErrNotFound) {
			return nil
		}
		return normalizeMetadataVFSError(err)
	}
	object.Status = entity.StorageObjectStatusMissing
	object.UpdatedAt = now
	if err := s.objectRepo.Update(ctx, object); err != nil {
		return normalizeMetadataVFSError(err)
	}
	return nil
}

func (s *MetadataVFSSyncService) markChildConflict(ctx context.Context, child *entity.VFSNode, now time.Time) error {
	if child == nil {
		return nil
	}
	child.SyncState = entity.VFSNodeSyncStateConflict
	child.UpdatedAt = now
	if err := s.nodeRepo.Update(ctx, child); err != nil {
		return normalizeMetadataVFSError(err)
	}
	return nil
}

func (s *MetadataVFSSyncService) markTargetSyncState(ctx context.Context, target *entity.VFSNode, state string) error {
	return s.withinSyncTx(ctx, func(txCtx context.Context) error {
		now := s.now()
		target.SyncState = state
		target.UpdatedAt = now
		if state == entity.VFSNodeSyncStateIndexed || state == entity.VFSNodeSyncStateConflict {
			target.IndexedAt = &now
			target.LastSeenAt = &now
		}
		if err := s.nodeRepo.Update(txCtx, target); err != nil {
			return normalizeMetadataVFSError(err)
		}
		return nil
	})
}

func (s *MetadataVFSSyncService) withinSyncTx(ctx context.Context, fn func(context.Context) error) error {
	if s.transactor == nil {
		return fn(ctx)
	}
	return s.transactor.WithinTx(ctx, fn)
}

type fileDriverRemoteIndexer struct {
	driver FileDriver
}

type localFilesystemRemoteIndexer struct{}

func (localFilesystemRemoteIndexer) ListRemoteChildren(_ context.Context, source *entity.StorageSource, req RemoteListRequest) ([]RemoteEntry, error) {
	if source == nil {
		return nil, ErrFileNotFound
	}
	_, physicalPath, err := resolvePhysicalPath(source, req.ParentPath)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(physicalPath)
	if err != nil {
		return nil, err
	}

	items := make([]RemoteEntry, 0, len(entries))
	for _, entry := range entries {
		entryPath := joinVirtualPath(req.ParentPath, entry.Name())
		if isHiddenVirtualPath(entryPath) {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil, infoErr
		}
		item := RemoteEntry{
			Name:       entry.Name(),
			Path:       entryPath,
			IsDir:      entry.IsDir(),
			ModifiedAt: info.ModTime(),
		}
		if !entry.IsDir() {
			item.Size = info.Size()
			item.ETag = buildEtag(info)
			item.MimeType = remoteEntryMimeType(item)
		}
		items = append(items, item)
	}
	return items, nil
}

func (i fileDriverRemoteIndexer) ListRemoteChildren(ctx context.Context, source *entity.StorageSource, req RemoteListRequest) ([]RemoteEntry, error) {
	if i.driver == nil {
		return nil, ErrSourceDriverUnsupported
	}
	entries, err := i.driver.List(ctx, source, req.ParentPath)
	if err != nil {
		return nil, err
	}
	items := make([]RemoteEntry, 0, len(entries))
	for _, entry := range entries {
		items = append(items, RemoteEntry{
			Name:             entry.Name,
			Path:             entry.Path,
			IsDir:            entry.IsDir,
			Size:             entry.Size,
			ETag:             entry.ETag,
			Checksum:         entry.Checksum,
			MimeType:         entry.MimeType,
			ModifiedAt:       entry.ModifiedAt,
			ProviderItemID:   entry.ProviderItemID,
			ProviderParentID: entry.ProviderParentID,
			LocatorType:      entry.LocatorType,
			LocatorJSON:      entry.LocatorJSON,
		})
	}
	return items, nil
}

func remoteEntryName(entry RemoteEntry) string {
	if entry.Name != "" {
		return entry.Name
	}
	if entry.Path == "" || entry.Path == "/" {
		return ""
	}
	return path.Base(entry.Path)
}

func normalizeRemoteEntryPath(parentPath string, entry RemoteEntry, name string) string {
	if entry.Path != "" {
		if normalized, err := normalizeVirtualPath(entry.Path); err == nil {
			return normalized
		}
	}
	if normalized, err := normalizeVirtualPath(parentPath); err == nil {
		return joinVirtualPath(normalized, name)
	}
	return "/" + strings.TrimPrefix(name, "/")
}

func metadataVFSRemoteEntryKind(entry RemoteEntry) string {
	if entry.IsDir {
		return entity.VFSNodeKindDir
	}
	return entity.VFSNodeKindFile
}

func remoteEntrySize(entry RemoteEntry) int64 {
	if entry.IsDir {
		return 0
	}
	if entry.Size < 0 {
		return 0
	}
	return entry.Size
}

func remoteEntryMimeType(entry RemoteEntry) string {
	if entry.IsDir {
		return "inode/directory"
	}
	if entry.MimeType != "" {
		return entry.MimeType
	}
	extension := strings.ToLower(filepath.Ext(remoteEntryName(entry)))
	if extension == "" {
		return "application/octet-stream"
	}
	mimeType := mime.TypeByExtension(extension)
	if mimeType == "" {
		return "application/octet-stream"
	}
	return mimeType
}

func metadataVFSRemoteEntryLocator(source *entity.StorageSource, entry RemoteEntry, entryPath string) (string, string, error) {
	locatorType := strings.TrimSpace(entry.LocatorType)
	locatorJSON := strings.TrimSpace(entry.LocatorJSON)
	if locatorType != "" && locatorJSON != "" {
		return locatorType, locatorJSON, nil
	}
	if locatorType == "" {
		switch {
		case entry.ProviderItemID != "":
			locatorType = "provider_file_id"
		case source != nil && source.DriverType == "local":
			locatorType = "local_path"
		default:
			locatorType = "provider_path"
		}
	}

	payload := map[string]string{}
	if entry.ProviderItemID != "" {
		payload["file_id"] = entry.ProviderItemID
	}
	if entry.ProviderParentID != "" {
		payload["parent_id"] = entry.ProviderParentID
	}
	if entryPath != "" {
		payload["path"] = entryPath
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", "", err
	}
	return locatorType, string(raw), nil
}

func providerParentIDForEntry(parent *entity.VFSNode, entry RemoteEntry) *string {
	if entry.ProviderParentID != "" {
		return stringPtrOrNil(entry.ProviderParentID)
	}
	if parent != nil && parent.ProviderItemID != nil {
		return cloneStringPtr(parent.ProviderItemID)
	}
	return nil
}

func stringPtrOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func metadataVFSRemoteIdentityConflicts(existing *entity.VFSNode, entry RemoteEntry) bool {
	if existing == nil || existing.ProviderItemID == nil || entry.ProviderItemID == "" {
		return false
	}
	return *existing.ProviderItemID != entry.ProviderItemID
}

func metadataVFSChildIsControlNode(node *entity.VFSNode) bool {
	return node != nil &&
		(node.Kind == entity.VFSNodeKindMount || node.Kind == entity.VFSNodeKindVirtualDir)
}

func metadataVFSNodeChangedByRemote(existing *entity.VFSNode, next *entity.VFSNode) bool {
	if existing == nil || next == nil {
		return existing != next
	}
	return existing.Kind != next.Kind ||
		!sameUintPtr(existing.ObjectID, next.ObjectID) ||
		!sameUintPtr(existing.SourceID, next.SourceID) ||
		!sameStringPtr(existing.ProviderItemID, next.ProviderItemID) ||
		!sameStringPtr(existing.ProviderParentID, next.ProviderParentID) ||
		existing.Size != next.Size ||
		existing.MimeType != next.MimeType ||
		existing.ETag != next.ETag ||
		existing.Checksum != next.Checksum ||
		existing.SyncState != next.SyncState ||
		existing.IsDeleted != next.IsDeleted
}

func sameStringPtr(left *string, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func normalizeMetadataVFSSyncProviderError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrSourceDriverUnsupported),
		errors.Is(err, ErrSourceOperationUnsupported),
		errors.Is(err, ErrCloudAuthFailed),
		errors.Is(err, ErrCloudTokenInvalid),
		errors.Is(err, ErrCloudCaptchaRequired),
		errors.Is(err, ErrCloudCaptchaExpired),
		errors.Is(err, ErrCloudRateLimited),
		errors.Is(err, ErrCloudRegionBlocked),
		errors.Is(err, ErrCloudProviderUnavailable):
		return err
	case errors.Is(err, os.ErrNotExist):
		return ErrFileNotFound
	case errors.Is(err, fs.ErrInvalid), errors.Is(err, os.ErrInvalid):
		return ErrPathInvalid
	default:
		return fmt.Errorf("%w: %v", ErrCloudProviderUnavailable, err)
	}
}
