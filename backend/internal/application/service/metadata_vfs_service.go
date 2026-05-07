package service

import (
	"context"
	"errors"
	"mime"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

// MetadataVFSService 提供以 DB VFS metadata 为主的控制面服务。
//
// 本阶段只更新 VFS metadata tree 的路径快照，并在已注入 objectRepo 时同步
// path-based storage object locator；不直接触碰底层 provider 数据面。
type MetadataVFSService struct {
	nodeRepo   domainrepo.VFSNodeRepository
	objectRepo domainrepo.StorageObjectRepository
	sourceRepo domainrepo.SourceRepository
	tagRepo    domainrepo.VFSTagRepository
	transactor domainrepo.Transactor
	now        func() time.Time
}

// MetadataVFSServiceOption 定义 MetadataVFSService 可选依赖。
type MetadataVFSServiceOption func(*MetadataVFSService)

// MetadataVFSMkdirRequest 定义元数据目录创建请求。
type MetadataVFSMkdirRequest struct {
	ParentPath string
	Name       string
	ActorID    *uint
}

// MetadataVFSRenameRequest 定义元数据重命名请求。
type MetadataVFSRenameRequest struct {
	Path    string
	NewName string
	ActorID *uint
}

// MetadataVFSMoveRequest 定义元数据移动请求。
type MetadataVFSMoveRequest struct {
	Path             string
	TargetParentPath string
	ActorID          *uint
}

// WithMetadataVFSTagRepository 注册 VFS tag 仓储。
func WithMetadataVFSTagRepository(tagRepo domainrepo.VFSTagRepository) MetadataVFSServiceOption {
	return func(s *MetadataVFSService) {
		s.tagRepo = tagRepo
	}
}

// WithMetadataVFSObjectLocatorSync 注册 path-based object locator 同步依赖。
func WithMetadataVFSObjectLocatorSync(sourceRepo domainrepo.SourceRepository, objectRepo domainrepo.StorageObjectRepository) MetadataVFSServiceOption {
	return func(s *MetadataVFSService) {
		s.sourceRepo = sourceRepo
		s.objectRepo = objectRepo
	}
}

// WithMetadataVFSTransactor 注册事务端口，用于批量 path 快照更新。
func WithMetadataVFSTransactor(transactor domainrepo.Transactor) MetadataVFSServiceOption {
	return func(s *MetadataVFSService) {
		s.transactor = transactor
	}
}

// WithMetadataVFSClock 覆盖当前时间，主要用于测试。
func WithMetadataVFSClock(now func() time.Time) MetadataVFSServiceOption {
	return func(s *MetadataVFSService) {
		if now != nil {
			s.now = now
		}
	}
}

// NewMetadataVFSService 创建 metadata VFS 控制面服务。
func NewMetadataVFSService(nodeRepo domainrepo.VFSNodeRepository, options ...MetadataVFSServiceOption) *MetadataVFSService {
	service := &MetadataVFSService{
		nodeRepo: nodeRepo,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// EnsureRoot 确保 metadata VFS 根节点存在。
func (s *MetadataVFSService) EnsureRoot(ctx context.Context) (*entity.VFSNode, error) {
	if s == nil || s.nodeRepo == nil {
		return nil, ErrSourceDriverUnsupported
	}

	now := s.now()
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

// ResolveNode 按虚拟 path 解析未删除的 VFS node。
func (s *MetadataVFSService) ResolveNode(ctx context.Context, virtualPath string) (*entity.VFSNode, error) {
	if s == nil || s.nodeRepo == nil {
		return nil, ErrSourceDriverUnsupported
	}
	normalizedPath, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return nil, err
	}
	node, err := s.nodeRepo.FindByPath(ctx, normalizedPath)
	if err != nil {
		return nil, normalizeMetadataVFSError(err)
	}
	return node, nil
}

// ResolveNodeByID 按 node id 解析未删除且可用的 VFS node。
func (s *MetadataVFSService) ResolveNodeByID(ctx context.Context, id uint) (*entity.VFSNode, error) {
	if s == nil || s.nodeRepo == nil {
		return nil, ErrSourceDriverUnsupported
	}
	node, err := s.nodeRepo.FindByID(ctx, id)
	if err != nil {
		return nil, normalizeMetadataVFSError(err)
	}
	if node.IsDeleted {
		return nil, ErrFileNotFound
	}
	return node, nil
}

// ListChildren 列出指定目录的 DB metadata 子节点。
func (s *MetadataVFSService) ListChildren(ctx context.Context, currentPath string) (*appdto.VFSListResponse, error) {
	parent, err := s.ResolveNode(ctx, currentPath)
	if err != nil {
		return nil, err
	}
	if !metadataVFSNodeIsDirectory(parent) {
		return nil, ErrPathInvalid
	}

	children, err := s.nodeRepo.ListChildren(ctx, &parent.ID, domainrepo.VFSNodeListFilter{})
	if err != nil {
		return nil, normalizeMetadataVFSError(err)
	}

	items := make([]appdto.VFSItem, 0, len(children))
	for _, child := range children {
		items = append(items, metadataVFSItemFromNode(child))
	}
	sortMetadataVFSItems(items)

	return &appdto.VFSListResponse{
		Items:       items,
		CurrentPath: parent.Path,
	}, nil
}

// Search 按 path prefix 限定 DB metadata 范围，并在 service 层做 name/path contains 匹配。
func (s *MetadataVFSService) Search(ctx context.Context, pathPrefix string, keyword string) (*appdto.VFSSearchResponse, error) {
	normalizedPrefix, err := normalizeVirtualPath(pathPrefix)
	if err != nil {
		return nil, err
	}
	rawKeyword := strings.TrimSpace(keyword)
	normalizedKeyword := strings.ToLower(rawKeyword)
	if normalizedKeyword == "" {
		return nil, ErrPathInvalid
	}
	prefixNode, err := s.ResolveNode(ctx, normalizedPrefix)
	if err != nil {
		return nil, err
	}

	nodes, err := s.nodeRepo.ListByPathPrefix(ctx, prefixNode.Path, domainrepo.VFSNodeListFilter{})
	if err != nil {
		return nil, normalizeMetadataVFSError(err)
	}

	items := make([]appdto.VFSItem, 0, len(nodes))
	for _, node := range nodes {
		if node.ID == prefixNode.ID {
			continue
		}
		if metadataVFSNodeMatchesKeyword(node, normalizedKeyword) {
			items = append(items, metadataVFSItemFromNode(node))
		}
	}
	sortMetadataVFSItems(items)

	return &appdto.VFSSearchResponse{
		Items:      items,
		PathPrefix: prefixNode.Path,
		Keyword:    rawKeyword,
	}, nil
}

// Mkdir 创建 metadata 目录：纯虚拟目录下创建 virtual_dir，挂载/有 source 的目录下创建 dir。
func (s *MetadataVFSService) Mkdir(ctx context.Context, req MetadataVFSMkdirRequest) (*appdto.VFSItem, error) {
	if err := validateFileName(req.Name); err != nil {
		return nil, err
	}
	parent, err := s.ResolveNode(ctx, req.ParentPath)
	if err != nil {
		return nil, err
	}
	if !metadataVFSNodeIsDirectory(parent) {
		return nil, ErrPathInvalid
	}

	targetPath := joinVirtualPath(parent.Path, req.Name)
	if err := s.ensureMetadataNameAvailable(ctx, targetPath); err != nil {
		return nil, err
	}

	now := s.now()
	node := &entity.VFSNode{
		ParentID:  &parent.ID,
		Name:      req.Name,
		Path:      targetPath,
		Kind:      metadataVFSChildDirKind(parent),
		MountID:   cloneUintPtr(parent.MountID),
		SourceID:  cloneUintPtr(parent.SourceID),
		SyncState: entity.VFSNodeSyncStateIndexed,
		CreatedBy: cloneUintPtr(req.ActorID),
		UpdatedBy: cloneUintPtr(req.ActorID),
		CreatedAt: now,
		UpdatedAt: now,
		IndexedAt: &now,
	}
	if err := s.nodeRepo.Create(ctx, node); err != nil {
		return nil, normalizeMetadataVFSError(err)
	}
	item := metadataVFSItemFromNode(node)
	return &item, nil
}

// Rename 重命名 metadata tree 内的节点，并更新子树 path 快照。
func (s *MetadataVFSService) Rename(ctx context.Context, req MetadataVFSRenameRequest) (string, *appdto.VFSItem, error) {
	if err := validateFileName(req.NewName); err != nil {
		return "", nil, err
	}
	node, err := s.ResolveNode(ctx, req.Path)
	if err != nil {
		return "", nil, err
	}
	if node.Kind == entity.VFSNodeKindRoot || node.ParentID == nil {
		return "", nil, ErrPathInvalid
	}
	if node.Kind == entity.VFSNodeKindMount {
		return "", nil, ErrSourceOperationUnsupported
	}
	if node.Name == req.NewName {
		item := metadataVFSItemFromNode(node)
		return node.Path, &item, nil
	}

	parentPath := path.Dir(node.Path)
	if parentPath == "." {
		parentPath = "/"
	}
	newPath := joinVirtualPath(parentPath, req.NewName)
	if err := s.ensureMetadataNameAvailable(ctx, newPath); err != nil {
		return "", nil, err
	}

	oldPath := node.Path
	if err := s.updateMetadataSubtreePath(ctx, node, *node.ParentID, req.NewName, newPath, req.ActorID); err != nil {
		return "", nil, err
	}
	updated, err := s.ResolveNode(ctx, newPath)
	if err != nil {
		return "", nil, err
	}
	item := metadataVFSItemFromNode(updated)
	return oldPath, &item, nil
}

// Move 移动 metadata tree 内的节点，并更新子树 path 快照。
func (s *MetadataVFSService) Move(ctx context.Context, req MetadataVFSMoveRequest) (string, *appdto.VFSItem, error) {
	node, err := s.ResolveNode(ctx, req.Path)
	if err != nil {
		return "", nil, err
	}
	if node.Kind == entity.VFSNodeKindRoot || node.ParentID == nil {
		return "", nil, ErrPathInvalid
	}
	if node.Kind == entity.VFSNodeKindMount {
		return "", nil, ErrSourceOperationUnsupported
	}

	targetParent, err := s.ResolveNode(ctx, req.TargetParentPath)
	if err != nil {
		return "", nil, err
	}
	if !metadataVFSNodeIsDirectory(targetParent) {
		return "", nil, ErrPathInvalid
	}
	if isSubPath(node.Path, targetParent.Path) {
		return "", nil, ErrPathInvalid
	}
	if !sameUintPtr(node.MountID, targetParent.MountID) || !sameUintPtr(node.SourceID, targetParent.SourceID) {
		return "", nil, ErrSourceOperationUnsupported
	}
	if node.ParentID != nil && *node.ParentID == targetParent.ID {
		item := metadataVFSItemFromNode(node)
		return node.Path, &item, nil
	}

	newPath := joinVirtualPath(targetParent.Path, node.Name)
	if err := s.ensureMetadataNameAvailable(ctx, newPath); err != nil {
		return "", nil, err
	}

	oldPath := node.Path
	if err := s.updateMetadataSubtreePath(ctx, node, targetParent.ID, node.Name, newPath, req.ActorID); err != nil {
		return "", nil, err
	}
	updated, err := s.ResolveNode(ctx, newPath)
	if err != nil {
		return "", nil, err
	}
	item := metadataVFSItemFromNode(updated)
	return oldPath, &item, nil
}

// Delete 软删除指定 metadata node 及其 path 子树。
func (s *MetadataVFSService) Delete(ctx context.Context, virtualPath string) (time.Time, error) {
	node, err := s.ResolveNode(ctx, virtualPath)
	if err != nil {
		return time.Time{}, err
	}
	if node.Kind == entity.VFSNodeKindRoot {
		return time.Time{}, ErrPathInvalid
	}
	if node.Kind == entity.VFSNodeKindMount {
		return time.Time{}, ErrSourceOperationUnsupported
	}
	deletedAt := s.now()
	if err := s.nodeRepo.Delete(ctx, node.ID); err != nil {
		return time.Time{}, normalizeMetadataVFSError(err)
	}
	return deletedAt, nil
}

// AttachTag 绑定已有 tag 到指定 metadata node。
func (s *MetadataVFSService) AttachTag(ctx context.Context, virtualPath string, tagID uint, actorID *uint) error {
	if s == nil || s.tagRepo == nil {
		return ErrSourceOperationUnsupported
	}
	node, err := s.ResolveNode(ctx, virtualPath)
	if err != nil {
		return err
	}
	if _, err := s.tagRepo.FindByID(ctx, tagID); err != nil {
		return normalizeMetadataVFSError(err)
	}
	if err := s.tagRepo.AttachToNode(ctx, &entity.VFSNodeTag{
		NodeID:    node.ID,
		TagID:     tagID,
		CreatedBy: cloneUintPtr(actorID),
		CreatedAt: s.now(),
	}); err != nil {
		return normalizeMetadataVFSError(err)
	}
	return nil
}

// ListTags 列出指定 metadata node 的标签。
func (s *MetadataVFSService) ListTags(ctx context.Context, virtualPath string) ([]*entity.VFSTag, error) {
	if s == nil || s.tagRepo == nil {
		return nil, ErrSourceOperationUnsupported
	}
	node, err := s.ResolveNode(ctx, virtualPath)
	if err != nil {
		return nil, err
	}
	tags, err := s.tagRepo.ListTagsForNode(ctx, node.ID)
	if err != nil {
		return nil, normalizeMetadataVFSError(err)
	}
	return tags, nil
}

func (s *MetadataVFSService) ensureMetadataNameAvailable(ctx context.Context, targetPath string) error {
	normalizedPath, err := normalizeVirtualPath(targetPath)
	if err != nil {
		return err
	}
	if normalizedPath == "/" {
		return ErrNameConflict
	}
	if _, err := s.nodeRepo.FindByPath(ctx, normalizedPath); err == nil {
		return ErrNameConflict
	} else if !errors.Is(err, domainrepo.ErrNotFound) {
		return normalizeMetadataVFSError(err)
	}
	return nil
}

func (s *MetadataVFSService) updateMetadataSubtreePath(ctx context.Context, node *entity.VFSNode, newParentID uint, newName string, newPath string, actorID *uint) error {
	return s.withinTx(ctx, func(txCtx context.Context) error {
		subtree, err := s.nodeRepo.ListByPathPrefix(txCtx, node.Path, domainrepo.VFSNodeListFilter{})
		if err != nil {
			return normalizeMetadataVFSError(err)
		}
		if len(subtree) == 0 {
			return ErrFileNotFound
		}
		sort.SliceStable(subtree, func(i, j int) bool {
			return len(subtree[i].Path) < len(subtree[j].Path)
		})
		if err := s.ensureMetadataSubtreeTargetAvailable(txCtx, subtree, node.Path, newPath); err != nil {
			return err
		}

		now := s.now()
		for _, item := range subtree {
			if item.ID == node.ID {
				item.ParentID = &newParentID
				item.Name = newName
				item.Path = newPath
			} else {
				item.Path = rewriteMetadataSubtreePath(item.Path, node.Path, newPath)
			}
			item.UpdatedBy = cloneUintPtr(actorID)
			item.UpdatedAt = now
			if err := s.syncPathBasedObjectLocator(txCtx, item, now); err != nil {
				return err
			}
			if err := s.nodeRepo.Update(txCtx, item); err != nil {
				return normalizeMetadataVFSError(err)
			}
		}
		return nil
	})
}

func (s *MetadataVFSService) syncPathBasedObjectLocator(ctx context.Context, node *entity.VFSNode, now time.Time) error {
	if s == nil || s.objectRepo == nil || s.sourceRepo == nil || node == nil ||
		node.Kind != entity.VFSNodeKindFile || node.ObjectID == nil || node.SourceID == nil {
		return nil
	}
	object, err := s.objectRepo.FindByID(ctx, *node.ObjectID)
	if err != nil {
		if errors.Is(err, domainrepo.ErrNotFound) {
			return nil
		}
		return normalizeMetadataVFSError(err)
	}
	if !metadataVFSObjectLocatorPathBased(object) {
		return nil
	}
	innerPath, err := s.metadataNodeInnerPath(ctx, *node.SourceID, node.Path)
	if err != nil {
		return err
	}
	locatorJSON, err := metadataCommitMarshalLocatorJSON(map[string]any{"path": innerPath})
	if err != nil {
		return err
	}
	object.LocatorJSON = locatorJSON
	object.UpdatedAt = now
	if err := s.objectRepo.Update(ctx, object); err != nil {
		return normalizeMetadataVFSError(err)
	}
	return nil
}

func metadataVFSObjectLocatorPathBased(object *entity.StorageObject) bool {
	if object == nil {
		return false
	}
	switch strings.TrimSpace(object.LocatorType) {
	case "local_path", "driver_path", "provider_path":
		return true
	default:
		return false
	}
}

func (s *MetadataVFSService) metadataNodeInnerPath(ctx context.Context, sourceID uint, virtualPath string) (string, error) {
	source, err := s.sourceRepo.FindByID(ctx, sourceID)
	if err != nil {
		return "", normalizeMetadataVFSError(err)
	}
	if source == nil || strings.TrimSpace(source.MountPath) == "" {
		return normalizeVirtualPath(virtualPath)
	}
	mountPath, err := normalizeMountPath(source.MountPath)
	if err != nil {
		return "", err
	}
	normalizedPath, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return "", err
	}
	if !isSubPath(mountPath, normalizedPath) {
		return "", ErrPathInvalid
	}
	inner := strings.TrimPrefix(normalizedPath, mountPath)
	if inner == "" {
		inner = "/"
	}
	if !strings.HasPrefix(inner, "/") {
		inner = "/" + inner
	}
	return normalizeVirtualPath(inner)
}

func (s *MetadataVFSService) ensureMetadataSubtreeTargetAvailable(ctx context.Context, subtree []*entity.VFSNode, oldPrefix string, newPrefix string) error {
	subtreeIDs := make(map[uint]struct{}, len(subtree))
	for _, item := range subtree {
		subtreeIDs[item.ID] = struct{}{}
	}

	for _, item := range subtree {
		targetPath := rewriteMetadataSubtreePath(item.Path, oldPrefix, newPrefix)
		existing, err := s.nodeRepo.FindByPath(ctx, targetPath)
		if err == nil {
			if _, ok := subtreeIDs[existing.ID]; ok {
				continue
			}
			return ErrNameConflict
		}
		if !errors.Is(err, domainrepo.ErrNotFound) {
			return normalizeMetadataVFSError(err)
		}
	}
	return nil
}

func (s *MetadataVFSService) withinTx(ctx context.Context, fn func(context.Context) error) error {
	if s.transactor == nil {
		return fn(ctx)
	}
	return s.transactor.WithinTx(ctx, fn)
}

func normalizeMetadataVFSError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, domainrepo.ErrNotFound):
		return ErrFileNotFound
	case errors.Is(err, domainrepo.ErrConflict), errors.Is(err, domainrepo.ErrConstraintViolation):
		return ErrNameConflict
	default:
		return err
	}
}

func metadataVFSChildDirKind(parent *entity.VFSNode) string {
	if parent == nil {
		return entity.VFSNodeKindVirtualDir
	}
	if parent.Kind == entity.VFSNodeKindMount || parent.SourceID != nil || parent.MountID != nil {
		return entity.VFSNodeKindDir
	}
	return entity.VFSNodeKindVirtualDir
}

func metadataVFSNodeIsDirectory(node *entity.VFSNode) bool {
	if node == nil {
		return false
	}
	return node.Kind == entity.VFSNodeKindRoot ||
		node.Kind == entity.VFSNodeKindVirtualDir ||
		node.Kind == entity.VFSNodeKindMount ||
		node.Kind == entity.VFSNodeKindDir
}

func metadataVFSNodeMatchesKeyword(node *entity.VFSNode, keyword string) bool {
	if node == nil {
		return false
	}
	name := strings.ToLower(node.Name)
	pathValue := strings.ToLower(node.Path)
	return strings.Contains(name, keyword) || strings.Contains(pathValue, keyword)
}

func metadataVFSItemFromNode(node *entity.VFSNode) appdto.VFSItem {
	itemPath := "/"
	name := ""
	nodeID := uint(0)
	if node != nil {
		nodeID = node.ID
		itemPath = node.Path
		name = node.Name
	}
	parentPath := path.Dir(itemPath)
	if parentPath == "." {
		parentPath = "/"
	}

	entryKind := string(VirtualEntryKindFile)
	if metadataVFSNodeIsDirectory(node) {
		entryKind = string(VirtualEntryKindDirectory)
	}

	mimeType := ""
	extension := ""
	size := int64(0)
	etag := ""
	createdAt := ""
	modifiedAt := ""
	sourceID := (*uint)(nil)
	canPreview := false
	canDownload := false
	canDelete := false
	isVirtual := false
	isMountPoint := false
	syncState := ""

	if node != nil {
		mimeType = node.MimeType
		size = node.Size
		etag = node.ETag
		sourceID = cloneUintPtr(node.SourceID)
		createdAt = node.CreatedAt.UTC().Format(time.RFC3339)
		modifiedAt = node.UpdatedAt.UTC().Format(time.RFC3339)
		syncState = node.SyncState
		isMountPoint = node.Kind == entity.VFSNodeKindMount
		isVirtual = node.Kind == entity.VFSNodeKindRoot ||
			node.Kind == entity.VFSNodeKindVirtualDir ||
			node.Kind == entity.VFSNodeKindMount
		canDelete = node.Kind != entity.VFSNodeKindRoot && node.Kind != entity.VFSNodeKindMount
	}

	if entryKind == string(VirtualEntryKindDirectory) {
		mimeType = "inode/directory"
		size = 0
	} else {
		extension = strings.ToLower(filepath.Ext(name))
		if mimeType == "" {
			mimeType = mime.TypeByExtension(extension)
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}
		}
		canPreview = canPreviewMIME(mimeType)
		canDownload = node != nil && !metadataVFSNodeUnavailable(node)
	}

	return appdto.VFSItem{
		ID:           nodeID,
		Name:         name,
		Path:         itemPath,
		ParentPath:   parentPath,
		SourceID:     sourceID,
		EntryKind:    entryKind,
		IsVirtual:    isVirtual,
		IsMountPoint: isMountPoint,
		SyncState:    syncState,
		Size:         size,
		MimeType:     mimeType,
		Extension:    extension,
		ModifiedAt:   modifiedAt,
		CreatedAt:    createdAt,
		Etag:         etag,
		CanPreview:   canPreview,
		CanDownload:  canDownload,
		CanDelete:    canDelete,
	}
}

func metadataVFSNodeUnavailable(node *entity.VFSNode) bool {
	if node == nil {
		return true
	}
	return node.SyncState == entity.VFSNodeSyncStateMissing ||
		node.SyncState == entity.VFSNodeSyncStateError ||
		node.SyncState == entity.VFSNodeSyncStatePending ||
		node.SyncState == entity.VFSNodeSyncStateSyncing ||
		node.SyncState == entity.VFSNodeSyncStateConflict ||
		node.IsDeleted
}

func rewriteMetadataSubtreePath(currentPath string, oldPrefix string, newPrefix string) string {
	if currentPath == oldPrefix {
		return newPrefix
	}
	suffix := strings.TrimPrefix(currentPath, oldPrefix)
	if suffix == "" {
		return newPrefix
	}
	if !strings.HasPrefix(suffix, "/") {
		suffix = "/" + suffix
	}
	return newPrefix + suffix
}

func sortMetadataVFSItems(items []appdto.VFSItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].EntryKind != items[j].EntryKind {
			return items[i].EntryKind == string(VirtualEntryKindDirectory)
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
}

func cloneUintPtr(value *uint) *uint {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func sameUintPtr(left *uint, right *uint) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
