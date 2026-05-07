package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"path"
	"strings"
	"time"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

// MetadataVFSCommitter 是业务模块写入 metadata VFS 控制面快照的应用层端口。
type MetadataVFSCommitter interface {
	CommitFileObject(ctx context.Context, req MetadataVFSFileObjectCommitRequest) (*MetadataVFSFileObjectCommitResult, error)
}

// MetadataVFSFileObjectCommitRequest 描述一次“文件节点 + 数据对象”的提交。
type MetadataVFSFileObjectCommitRequest struct {
	Source *entity.StorageSource

	SourceID   uint
	DriverType string

	VirtualParentPath       string
	ResolvedInnerParentPath string
	Filename                string
	ObjectPath              string

	LocatorType string
	LocatorJSON string

	Size     int64
	MimeType string
	ETag     string
	Checksum string
	ActorID  *uint
}

// MetadataVFSFileObjectCommitResult 返回本次提交后的 node/object。
type MetadataVFSFileObjectCommitResult struct {
	Node   *entity.VFSNode
	Object *entity.StorageObject
}

// MetadataVFSCommitService 负责把已成功落地的数据面文件提交为 metadata VFS 快照。
type MetadataVFSCommitService struct {
	nodeRepo   domainrepo.VFSNodeRepository
	objectRepo domainrepo.StorageObjectRepository
	transactor domainrepo.Transactor
	now        func() time.Time
}

// MetadataVFSCommitServiceOption 定义 MetadataVFSCommitService 可选依赖。
type MetadataVFSCommitServiceOption func(*MetadataVFSCommitService)

// WithMetadataVFSCommitTransactor 注册事务端口，保证 parent/node/object 同批提交。
func WithMetadataVFSCommitTransactor(transactor domainrepo.Transactor) MetadataVFSCommitServiceOption {
	return func(s *MetadataVFSCommitService) {
		s.transactor = transactor
	}
}

// WithMetadataVFSCommitClock 覆盖当前时间，主要用于测试。
func WithMetadataVFSCommitClock(now func() time.Time) MetadataVFSCommitServiceOption {
	return func(s *MetadataVFSCommitService) {
		if now != nil {
			s.now = now
		}
	}
}

// NewMetadataVFSCommitService 创建 metadata VFS 提交服务。
func NewMetadataVFSCommitService(
	nodeRepo domainrepo.VFSNodeRepository,
	objectRepo domainrepo.StorageObjectRepository,
	options ...MetadataVFSCommitServiceOption,
) *MetadataVFSCommitService {
	service := &MetadataVFSCommitService{
		nodeRepo:   nodeRepo,
		objectRepo: objectRepo,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// CommitFileObject 幂等创建/更新文件 node 与 storage object，并懒创建父目录路径。
func (s *MetadataVFSCommitService) CommitFileObject(ctx context.Context, req MetadataVFSFileObjectCommitRequest) (*MetadataVFSFileObjectCommitResult, error) {
	if s == nil || s.nodeRepo == nil || s.objectRepo == nil {
		return nil, ErrSourceDriverUnsupported
	}
	if err := validateFileName(req.Filename); err != nil {
		return nil, err
	}

	sourceID, driverType, err := metadataCommitSourceSnapshot(req)
	if err != nil {
		return nil, err
	}
	virtualParentPath, err := metadataCommitVirtualParentPath(req)
	if err != nil {
		return nil, err
	}
	innerParentPath, err := metadataCommitInnerParentPath(req, virtualParentPath)
	if err != nil {
		return nil, err
	}
	objectPath, err := metadataCommitObjectPath(req, innerParentPath)
	if err != nil {
		return nil, err
	}
	locatorType, locatorJSON, err := metadataCommitLocator(req, driverType, objectPath)
	if err != nil {
		return nil, err
	}

	var result MetadataVFSFileObjectCommitResult
	err = s.withinCommitTx(ctx, func(txCtx context.Context) error {
		now := s.now()
		parent, err := s.ensureCommitParentPath(txCtx, req.Source, sourceID, virtualParentPath, now, req.ActorID)
		if err != nil {
			return err
		}

		filePath := joinVirtualPath(parent.Path, req.Filename)
		existing, err := s.nodeRepo.FindByPath(txCtx, filePath)
		if err == nil && existing.Kind != entity.VFSNodeKindFile {
			return ErrNameConflict
		}
		if err != nil && !errors.Is(err, domainrepo.ErrNotFound) {
			return normalizeMetadataVFSError(err)
		}

		object := &entity.StorageObject{
			SourceID:    sourceID,
			DriverType:  driverType,
			LocatorType: locatorType,
			LocatorJSON: locatorJSON,
			Size:        metadataCommitSize(req.Size),
			ETag:        req.ETag,
			Checksum:    req.Checksum,
			MimeType:    metadataCommitMimeType(req.Filename, req.MimeType),
			Status:      entity.StorageObjectStatusAvailable,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.objectRepo.UpsertByLocator(txCtx, object); err != nil {
			return normalizeMetadataVFSError(err)
		}

		node := &entity.VFSNode{
			ParentID:   &parent.ID,
			Name:       req.Filename,
			Path:       filePath,
			Kind:       entity.VFSNodeKindFile,
			MountID:    cloneUintPtr(parent.MountID),
			ObjectID:   &object.ID,
			SourceID:   uintPtr(sourceID),
			Size:       object.Size,
			MimeType:   object.MimeType,
			ETag:       req.ETag,
			Checksum:   req.Checksum,
			SyncState:  entity.VFSNodeSyncStateIndexed,
			CreatedBy:  cloneUintPtr(req.ActorID),
			UpdatedBy:  cloneUintPtr(req.ActorID),
			CreatedAt:  now,
			UpdatedAt:  now,
			IndexedAt:  &now,
			LastSeenAt: &now,
		}
		if existing != nil {
			node.ID = existing.ID
			node.CreatedAt = existing.CreatedAt
			node.CreatedBy = cloneUintPtr(existing.CreatedBy)
		}
		if err := s.nodeRepo.UpsertByPath(txCtx, node); err != nil {
			return normalizeMetadataVFSError(err)
		}

		result.Node = node
		result.Object = object
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *MetadataVFSCommitService) ensureCommitParentPath(
	ctx context.Context,
	source *entity.StorageSource,
	sourceID uint,
	virtualParentPath string,
	now time.Time,
	actorID *uint,
) (*entity.VFSNode, error) {
	root, err := s.ensureCommitRoot(ctx, now)
	if err != nil {
		return nil, err
	}
	if virtualParentPath == "/" {
		return root, nil
	}

	current := root
	segments := strings.Split(strings.TrimPrefix(virtualParentPath, "/"), "/")
	for _, segment := range segments {
		if segment == "" {
			continue
		}
		currentPath := joinVirtualPath(current.Path, segment)
		desiredKind := metadataCommitDirectoryKind(source, currentPath)
		desiredSourceID := metadataCommitDirectorySourceID(source, sourceID, currentPath)

		existing, err := s.nodeRepo.FindByPath(ctx, currentPath)
		if err == nil {
			if !metadataVFSNodeIsDirectory(existing) {
				return nil, ErrNameConflict
			}
			if existing.SourceID != nil && desiredSourceID != nil && *existing.SourceID != *desiredSourceID {
				return nil, ErrNameConflict
			}
			if metadataCommitDirectoryNeedsUpdate(existing, current.ID, segment, desiredKind, desiredSourceID) {
				existing.ParentID = &current.ID
				existing.Name = segment
				existing.Kind = desiredKind
				existing.SourceID = cloneUintPtr(desiredSourceID)
				existing.UpdatedBy = cloneUintPtr(actorID)
				existing.UpdatedAt = now
				existing.SyncState = entity.VFSNodeSyncStateIndexed
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
			MountID:    cloneUintPtr(current.MountID),
			SourceID:   cloneUintPtr(desiredSourceID),
			MimeType:   "inode/directory",
			SyncState:  entity.VFSNodeSyncStateIndexed,
			CreatedBy:  cloneUintPtr(actorID),
			UpdatedBy:  cloneUintPtr(actorID),
			CreatedAt:  now,
			UpdatedAt:  now,
			IndexedAt:  &now,
			LastSeenAt: &now,
		}
		if err := s.nodeRepo.UpsertByPath(ctx, node); err != nil {
			return nil, normalizeMetadataVFSError(err)
		}
		current = node
	}
	return current, nil
}

func (s *MetadataVFSCommitService) ensureCommitRoot(ctx context.Context, now time.Time) (*entity.VFSNode, error) {
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

func (s *MetadataVFSCommitService) withinCommitTx(ctx context.Context, fn func(context.Context) error) error {
	if s.transactor == nil {
		return fn(ctx)
	}
	return s.transactor.WithinTx(ctx, fn)
}

func metadataCommitSourceSnapshot(req MetadataVFSFileObjectCommitRequest) (uint, string, error) {
	sourceID := req.SourceID
	driverType := strings.TrimSpace(req.DriverType)
	if req.Source != nil {
		if sourceID == 0 {
			sourceID = req.Source.ID
		}
		if driverType == "" {
			driverType = req.Source.DriverType
		}
	}
	if sourceID == 0 || driverType == "" {
		return 0, "", ErrPathInvalid
	}
	return sourceID, driverType, nil
}

func metadataCommitVirtualParentPath(req MetadataVFSFileObjectCommitRequest) (string, error) {
	if strings.TrimSpace(req.VirtualParentPath) != "" {
		return normalizeVirtualPath(req.VirtualParentPath)
	}
	if req.Source != nil && strings.TrimSpace(req.ResolvedInnerParentPath) != "" {
		if merged := mergeMountAndInnerPath(req.Source.MountPath, req.ResolvedInnerParentPath); merged != "" {
			return merged, nil
		}
	}
	if strings.TrimSpace(req.ObjectPath) != "" && req.Source != nil {
		parentPath, _, err := splitParentName(req.ObjectPath)
		if err != nil {
			return "", err
		}
		if merged := mergeMountAndInnerPath(req.Source.MountPath, parentPath); merged != "" {
			return merged, nil
		}
	}
	return "", ErrPathInvalid
}

func metadataCommitInnerParentPath(req MetadataVFSFileObjectCommitRequest, virtualParentPath string) (string, error) {
	if strings.TrimSpace(req.ResolvedInnerParentPath) != "" {
		return normalizeVirtualPath(req.ResolvedInnerParentPath)
	}
	if req.Source != nil && strings.TrimSpace(req.Source.MountPath) != "" {
		mountPath, err := normalizeMountPath(req.Source.MountPath)
		if err == nil && isSubPath(mountPath, virtualParentPath) {
			inner := strings.TrimPrefix(virtualParentPath, mountPath)
			if inner == "" {
				return "/", nil
			}
			if !strings.HasPrefix(inner, "/") {
				inner = "/" + inner
			}
			return normalizeVirtualPath(inner)
		}
	}
	return normalizeVirtualPath(virtualParentPath)
}

func metadataCommitObjectPath(req MetadataVFSFileObjectCommitRequest, innerParentPath string) (string, error) {
	if strings.TrimSpace(req.ObjectPath) != "" {
		return normalizeVirtualPath(req.ObjectPath)
	}
	return normalizeVirtualPath(joinVirtualPath(innerParentPath, req.Filename))
}

func metadataCommitLocator(req MetadataVFSFileObjectCommitRequest, driverType string, objectPath string) (string, string, error) {
	locatorType := strings.TrimSpace(req.LocatorType)
	locatorJSON := strings.TrimSpace(req.LocatorJSON)
	if locatorType != "" && locatorJSON != "" {
		canonicalLocatorJSON, err := metadataCommitCanonicalLocatorJSON(locatorJSON)
		if err != nil {
			return "", "", err
		}
		return locatorType, canonicalLocatorJSON, nil
	}
	if locatorType == "" {
		if driverType == "local" {
			locatorType = "local_path"
		} else {
			locatorType = "driver_path"
		}
	}

	locatorJSON, err := metadataCommitMarshalLocatorJSON(map[string]any{"path": objectPath})
	return locatorType, locatorJSON, err
}

func metadataCommitCanonicalLocatorJSON(input string) (string, error) {
	var value any
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", ErrPathInvalid
	}
	if _, ok := value.(map[string]any); !ok {
		return "", ErrPathInvalid
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return "", ErrPathInvalid
	}
	return metadataCommitMarshalLocatorJSON(value)
}

func metadataCommitMarshalLocatorJSON(value any) (string, error) {
	raw, err := json.Marshal(metadataCommitSanitizeLocatorValue(value))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func metadataCommitSanitizeLocatorValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		sanitized := make(map[string]any, len(typed))
		for key, child := range typed {
			if metadataCommitLocatorKeyIsSensitive(key) || metadataCommitLocatorKeyIsPhysical(key) {
				sanitized[key] = "[redacted]"
				continue
			}
			sanitized[key] = metadataCommitSanitizeLocatorValue(child)
		}
		return sanitized
	case []any:
		sanitized := make([]any, 0, len(typed))
		for _, child := range typed {
			sanitized = append(sanitized, metadataCommitSanitizeLocatorValue(child))
		}
		return sanitized
	default:
		return value
	}
}

func metadataCommitLocatorKeyIsSensitive(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("-", "_", " ", "_").Replace(strings.TrimSpace(key)))
	switch normalized {
	case "access_token", "refresh_token", "id_token", "auth_token", "token",
		"password", "passwd", "secret", "client_secret", "secret_key",
		"api_key", "private_key", "credential", "credentials",
		"authorization", "cookie", "session", "session_id":
		return true
	default:
		return strings.HasSuffix(normalized, "_secret") ||
			strings.HasSuffix(normalized, "_token") ||
			strings.HasSuffix(normalized, "_password")
	}
}

func metadataCommitLocatorKeyIsPhysical(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("-", "_", " ", "_").Replace(strings.TrimSpace(key)))
	switch normalized {
	case "physical_path", "absolute_path", "base_path", "filesystem_path", "fs_path":
		return true
	default:
		return strings.HasSuffix(normalized, "_physical_path") ||
			strings.HasSuffix(normalized, "_absolute_path")
	}
}

func metadataCommitDirectoryKind(source *entity.StorageSource, currentPath string) string {
	if source == nil || strings.TrimSpace(source.MountPath) == "" {
		return entity.VFSNodeKindDir
	}
	mountPath, err := normalizeMountPath(source.MountPath)
	if err != nil {
		return entity.VFSNodeKindVirtualDir
	}
	if currentPath == mountPath {
		return entity.VFSNodeKindMount
	}
	if isSubPath(mountPath, currentPath) {
		return entity.VFSNodeKindDir
	}
	return entity.VFSNodeKindVirtualDir
}

func metadataCommitDirectorySourceID(source *entity.StorageSource, sourceID uint, currentPath string) *uint {
	if sourceID == 0 {
		return nil
	}
	if source == nil || strings.TrimSpace(source.MountPath) == "" {
		return uintPtr(sourceID)
	}
	mountPath, err := normalizeMountPath(source.MountPath)
	if err != nil {
		return nil
	}
	if isSubPath(mountPath, currentPath) {
		return uintPtr(sourceID)
	}
	return nil
}

func metadataCommitDirectoryNeedsUpdate(existing *entity.VFSNode, parentID uint, name string, kind string, sourceID *uint) bool {
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

func metadataCommitSize(size int64) int64 {
	if size < 0 {
		return 0
	}
	return size
}

func metadataCommitMimeType(filename string, mimeType string) string {
	if strings.TrimSpace(mimeType) != "" {
		return strings.TrimSpace(mimeType)
	}
	if detected := mime.TypeByExtension(strings.ToLower(path.Ext(filename))); detected != "" {
		return detected
	}
	return "application/octet-stream"
}

func uintPtr(value uint) *uint {
	if value == 0 {
		return nil
	}
	return &value
}
