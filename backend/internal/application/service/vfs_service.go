package service

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"time"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
)

type vfsFileOperator interface {
	Mkdir(ctx context.Context, req appdto.MkdirRequest) (*appdto.FileItem, error)
	Rename(ctx context.Context, req appdto.RenameRequest) (string, string, *appdto.FileItem, error)
	Move(ctx context.Context, req appdto.MoveCopyRequest) (string, string, error)
	Copy(ctx context.Context, req appdto.MoveCopyRequest) (string, string, error)
	Delete(ctx context.Context, req appdto.DeleteFileRequest) (time.Time, error)
}

type unsupportedVFSFileOperator struct{}

func (unsupportedVFSFileOperator) Mkdir(context.Context, appdto.MkdirRequest) (*appdto.FileItem, error) {
	return nil, ErrSourceDriverUnsupported
}

func (unsupportedVFSFileOperator) Rename(context.Context, appdto.RenameRequest) (string, string, *appdto.FileItem, error) {
	return "", "", nil, ErrSourceDriverUnsupported
}

func (unsupportedVFSFileOperator) Move(context.Context, appdto.MoveCopyRequest) (string, string, error) {
	return "", "", ErrSourceDriverUnsupported
}

func (unsupportedVFSFileOperator) Copy(context.Context, appdto.MoveCopyRequest) (string, string, error) {
	return "", "", ErrSourceDriverUnsupported
}

func (unsupportedVFSFileOperator) Delete(context.Context, appdto.DeleteFileRequest) (time.Time, error) {
	return time.Time{}, ErrSourceDriverUnsupported
}

type metadataVFSReader interface {
	EnsureRoot(ctx context.Context) (*entity.VFSNode, error)
	ResolveNode(ctx context.Context, virtualPath string) (*entity.VFSNode, error)
	ListChildren(ctx context.Context, currentPath string) (*appdto.VFSListResponse, error)
	Search(ctx context.Context, pathPrefix string, keyword string) (*appdto.VFSSearchResponse, error)
}

type metadataVFSRefreshService interface {
	RefreshPath(ctx context.Context, targetPath string) (*MetadataVFSRefreshResult, error)
}

// VFSService 提供统一虚拟目录树的路径解析能力。
type VFSService struct {
	registry            *MountRegistry
	fileDrivers         map[string]FileDriver
	capabilityProviders map[string]CapabilityProvider
	fileOp              vfsFileOperator
	aclAuthorizer       *ACLAuthorizer
	metadataReader      metadataVFSReader
	metadataRefresh     metadataVFSRefreshService
	localDirWritable    func(string) bool
}

// VFSServiceOption 定义 VFSService 的可选配置。
type VFSServiceOption func(*VFSService)

// WithVFSFileDriver 注册 VFSService 可复用的文件驱动。
func WithVFSFileDriver(driverType string, driver FileDriver) VFSServiceOption {
	return func(s *VFSService) {
		if driverType == "" || driver == nil {
			return
		}
		if s.fileDrivers == nil {
			s.fileDrivers = make(map[string]FileDriver)
		}
		s.fileDrivers[driverType] = driver
	}
}

// WithVFSCapabilityProvider 注册 VFSService 使用的 driver 能力描述器。
func WithVFSCapabilityProvider(driverType string, provider CapabilityProvider) VFSServiceOption {
	return func(s *VFSService) {
		if driverType == "" || provider == nil {
			return
		}
		if s.capabilityProviders == nil {
			s.capabilityProviders = make(map[string]CapabilityProvider)
		}
		s.capabilityProviders[driverType] = provider
	}
}

// WithVFSFileOperator 注册 VFSService 使用的底层文件操作器。
func WithVFSFileOperator(fileOp vfsFileOperator) VFSServiceOption {
	return func(s *VFSService) {
		s.fileOp = fileOp
	}
}

// WithVFSACLAuthorizer 注册 VFSService 使用的 ACL 判定器。
func WithVFSACLAuthorizer(authorizer *ACLAuthorizer) VFSServiceOption {
	return func(s *VFSService) {
		s.aclAuthorizer = authorizer
	}
}

// WithVFSMetadataServices 注册 metadata VFS 读模型与懒刷新服务。
func WithVFSMetadataServices(reader metadataVFSReader, refresh metadataVFSRefreshService) VFSServiceOption {
	return func(s *VFSService) {
		s.metadataReader = reader
		s.metadataRefresh = refresh
	}
}

// WithVFSLocalDirWritable 覆盖本地目录可写探测能力，主要用于测试只读挂载。
func WithVFSLocalDirWritable(checker func(string) bool) VFSServiceOption {
	return func(s *VFSService) {
		if checker != nil {
			s.localDirWritable = checker
		}
	}
}

// NewVFSService 创建 VFS 服务。
func NewVFSService(sourceRepo mountRegistrySourceRepository, options ...VFSServiceOption) *VFSService {
	service := &VFSService{
		registry:            NewMountRegistry(sourceRepo),
		fileDrivers:         make(map[string]FileDriver),
		capabilityProviders: make(map[string]CapabilityProvider),
		localDirWritable:    probeLocalDirectoryWritable,
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// ResolveWritableTarget 解析可写目标。
func (s *VFSService) ResolveWritableTarget(ctx context.Context, virtualPath string) (ResolvedPath, error) {
	mounts, err := s.registry.ListEnabledMounts(ctx)
	if err != nil {
		return ResolvedPath{}, err
	}
	if err := ensureWritableNameAvailable(ctx, virtualPath, mounts, s.fileDrivers); err != nil {
		return ResolvedPath{}, err
	}

	resolved, err := resolveVirtualPathByLongestPrefix(virtualPath, mounts)
	if err != nil {
		return ResolvedPath{}, err
	}
	if !resolved.IsRealMount {
		return ResolvedPath{}, ErrNoBackingStorage
	}

	return resolved, nil
}

// Mkdir 在统一虚拟目录树中创建目录。
func (s *VFSService) Mkdir(ctx context.Context, req appdto.VFSMkdirRequest) (*appdto.VFSItem, error) {
	targetPath := joinVirtualPath(req.ParentPath, req.Name)
	resolved, err := s.ResolveWritableTarget(ctx, targetPath)
	if err != nil {
		return nil, err
	}

	parentInnerPath, name, err := splitParentName(resolved.InnerPath)
	if err != nil {
		return nil, err
	}

	item, err := s.requireFileOperator().Mkdir(ctx, appdto.MkdirRequest{
		SourceID:   resolved.Source.ID,
		ParentPath: parentInnerPath,
		Name:       name,
	})
	if err != nil {
		return nil, normalizeVFSWriteError(err)
	}

	view := rewriteFileItemToVFSItem(resolved.MatchedMountPath, *item)
	return &view, nil
}

// Rename 在统一虚拟目录树中重命名节点。
func (s *VFSService) Rename(ctx context.Context, req appdto.VFSRenameRequest) (string, string, *appdto.VFSItem, error) {
	resolvedPath, err := s.ResolvePath(ctx, req.Path)
	if err != nil {
		return "", "", nil, err
	}

	parentVirtualPath := path.Dir(resolvedPath.VirtualPath)
	if parentVirtualPath == "." {
		parentVirtualPath = "/"
	}
	newVirtualPath := joinVirtualPath(parentVirtualPath, req.NewName)
	if _, err := s.ResolveWritableTarget(ctx, newVirtualPath); err != nil {
		return "", "", nil, err
	}

	oldPath, newPath, item, err := s.requireFileOperator().Rename(ctx, appdto.RenameRequest{
		SourceID: resolvedPath.Source.ID,
		Path:     resolvedPath.InnerPath,
		NewName:  req.NewName,
	})
	if err != nil {
		return "", "", nil, normalizeVFSWriteError(err)
	}

	virtualOldPath := mergeMountAndInnerPath(resolvedPath.MatchedMountPath, oldPath)
	virtualNewPath := mergeMountAndInnerPath(resolvedPath.MatchedMountPath, newPath)
	view := rewriteFileItemToVFSItem(resolvedPath.MatchedMountPath, *item)
	return virtualOldPath, virtualNewPath, &view, nil
}

// Move 在统一虚拟目录树中移动节点。
func (s *VFSService) Move(ctx context.Context, req appdto.VFSMoveCopyRequest) (string, string, error) {
	return s.moveOrCopy(ctx, req, true)
}

// Copy 在统一虚拟目录树中复制节点。
func (s *VFSService) Copy(ctx context.Context, req appdto.VFSMoveCopyRequest) (string, string, error) {
	return s.moveOrCopy(ctx, req, false)
}

// Delete 在统一虚拟目录树中删除节点。
func (s *VFSService) Delete(ctx context.Context, req appdto.VFSDeleteRequest) (time.Time, error) {
	resolvedPath, err := s.ResolvePath(ctx, req.Path)
	if err != nil {
		return time.Time{}, err
	}

	deletedAt, err := s.requireFileOperator().Delete(ctx, appdto.DeleteFileRequest{
		SourceID:   resolvedPath.Source.ID,
		Path:       resolvedPath.InnerPath,
		DeleteMode: req.DeleteMode,
	})
	if err != nil {
		return time.Time{}, normalizeVFSWriteError(err)
	}
	return deletedAt, nil
}

// ResolvePath 将统一虚拟路径解析到真实挂载源。
func (s *VFSService) ResolvePath(ctx context.Context, virtualPath string) (ResolvedPath, error) {
	mounts, err := s.registry.ListEnabledMounts(ctx)
	if err != nil {
		return ResolvedPath{}, err
	}

	resolved, err := resolveVirtualPathByLongestPrefix(virtualPath, mounts)
	if err != nil {
		return ResolvedPath{}, err
	}
	if !resolved.IsRealMount {
		return ResolvedPath{}, ErrFileNotFound
	}

	return resolved, nil
}

// List 列出统一虚拟目录树中的当前目录内容。
func (s *VFSService) List(ctx context.Context, currentPath string) (*appdto.VFSListResponse, error) {
	if s.metadataReader != nil {
		return s.listMetadata(ctx, currentPath)
	}
	return s.listLegacy(ctx, currentPath)
}

// Search 在统一虚拟目录树中搜索 metadata VFS 读模型。
func (s *VFSService) Search(ctx context.Context, pathPrefix string, keyword string) (*appdto.VFSSearchResponse, error) {
	if s.metadataReader == nil {
		return nil, ErrSourceDriverUnsupported
	}
	normalizedPrefix, err := normalizeVirtualPath(pathPrefix)
	if err != nil {
		return nil, err
	}
	if err := s.ensureMetadataRoot(ctx); err != nil {
		return nil, err
	}
	prefixNode, err := s.metadataReader.ResolveNode(ctx, normalizedPrefix)
	if err != nil {
		return nil, err
	}
	if metadataVFSNodeIsDirectory(prefixNode) {
		_ = s.refreshMetadataPathBestEffort(ctx, prefixNode)
	}

	resp, err := s.metadataReader.Search(ctx, normalizedPrefix, keyword)
	if err != nil {
		return nil, err
	}
	filtered, err := s.filterMetadataVFSItems(ctx, resp.Items)
	if err != nil {
		return nil, err
	}
	sortMetadataVFSItems(filtered)
	resp.Items = filtered
	return resp, nil
}

func (s *VFSService) listLegacy(ctx context.Context, currentPath string) (*appdto.VFSListResponse, error) {
	normalizedCurrentPath, err := normalizeVirtualPath(currentPath)
	if err != nil {
		return nil, err
	}

	mounts, err := s.registry.ListEnabledMounts(ctx)
	if err != nil {
		return nil, err
	}
	visibleMounts, err := s.filterVisibleMounts(ctx, mounts)
	if err != nil {
		return nil, err
	}
	projectedDirs, err := projectVirtualDirsFromMounts(normalizedCurrentPath, visibleMounts)
	if err != nil {
		return nil, err
	}

	merged := make(map[string]appdto.VFSItem)
	resolved, err := resolveVirtualPathByLongestPrefix(normalizedCurrentPath, visibleMounts)
	if err != nil {
		return nil, err
	}
	if resolved.IsRealMount {
		realItems, listErr := s.listMountedDirectory(ctx, normalizedCurrentPath, resolved)
		if listErr != nil {
			return nil, listErr
		}
		realItems, listErr = s.filterReadableVFSItems(ctx, resolved.Source.ID, realItems)
		if listErr != nil {
			return nil, listErr
		}
		for _, item := range realItems {
			merged[item.Name] = item
		}
	}

	for _, dir := range projectedDirs {
		merged[dir.Name] = buildVirtualDirItem(dir.Path, dir.IsMountPoint)
	}

	items := make([]appdto.VFSItem, 0, len(merged))
	for _, item := range merged {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})

	return &appdto.VFSListResponse{
		Items:       items,
		CurrentPath: normalizedCurrentPath,
	}, nil
}

func (s *VFSService) listMetadata(ctx context.Context, currentPath string) (*appdto.VFSListResponse, error) {
	normalizedCurrentPath, err := normalizeVirtualPath(currentPath)
	if err != nil {
		return nil, err
	}
	if err := s.ensureMetadataRoot(ctx); err != nil {
		return nil, err
	}

	currentNode, err := s.metadataReader.ResolveNode(ctx, normalizedCurrentPath)
	if err != nil {
		return nil, err
	}
	if !metadataVFSNodeIsDirectory(currentNode) {
		return nil, ErrPathInvalid
	}

	_ = s.refreshMetadataPathBestEffort(ctx, currentNode)
	resp, err := s.metadataReader.ListChildren(ctx, currentNode.Path)
	if err != nil {
		return nil, err
	}
	filtered, err := s.filterMetadataVFSItems(ctx, resp.Items)
	if err != nil {
		return nil, err
	}
	sortMetadataVFSItems(filtered)
	resp.Items = filtered
	resp.CurrentPath = normalizedCurrentPath
	return resp, nil
}

func (s *VFSService) moveOrCopy(ctx context.Context, req appdto.VFSMoveCopyRequest, removeSource bool) (string, string, error) {
	sourceResolved, err := s.ResolvePath(ctx, req.Path)
	if err != nil {
		return "", "", err
	}

	targetFilePath := joinVirtualPath(req.TargetPath, path.Base(sourceResolved.VirtualPath))
	targetResolved, err := s.ResolveWritableTarget(ctx, targetFilePath)
	if err != nil {
		return "", "", err
	}

	if sourceResolved.Source.ID == targetResolved.Source.ID {
		targetParentPath, _, splitErr := splitParentName(targetResolved.InnerPath)
		if splitErr != nil {
			return "", "", splitErr
		}
		if removeSource {
			oldPath, newPath, moveErr := s.requireFileOperator().Move(ctx, appdto.MoveCopyRequest{
				SourceID:   sourceResolved.Source.ID,
				Path:       sourceResolved.InnerPath,
				TargetPath: targetParentPath,
			})
			if moveErr != nil {
				return "", "", normalizeVFSWriteError(moveErr)
			}
			return mergeMountAndInnerPath(sourceResolved.MatchedMountPath, oldPath), mergeMountAndInnerPath(targetResolved.MatchedMountPath, newPath), nil
		}

		sourcePath, newPath, copyErr := s.requireFileOperator().Copy(ctx, appdto.MoveCopyRequest{
			SourceID:   sourceResolved.Source.ID,
			Path:       sourceResolved.InnerPath,
			TargetPath: targetParentPath,
		})
		if copyErr != nil {
			return "", "", normalizeVFSWriteError(copyErr)
		}
		return mergeMountAndInnerPath(sourceResolved.MatchedMountPath, sourcePath), mergeMountAndInnerPath(targetResolved.MatchedMountPath, newPath), nil
	}

	oldPath, newPath, err := s.copyAcrossSources(sourceResolved, targetResolved)
	if err != nil {
		return "", "", err
	}
	if removeSource {
		if err := s.removeLocalResolvedPath(sourceResolved); err != nil {
			return "", "", err
		}
	}
	return oldPath, newPath, nil
}

func (s *VFSService) copyAcrossSources(sourceResolved ResolvedPath, targetResolved ResolvedPath) (string, string, error) {
	if sourceResolved.Source == nil || targetResolved.Source == nil {
		return "", "", ErrFileNotFound
	}
	if sourceResolved.Source.DriverType != "local" || targetResolved.Source.DriverType != "local" {
		return "", "", ErrSourceDriverUnsupported
	}

	_, sourcePhysicalPath, err := resolvePhysicalPath(sourceResolved.Source, sourceResolved.InnerPath)
	if err != nil {
		return "", "", err
	}
	sourceInfo, err := os.Stat(sourcePhysicalPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", ErrFileNotFound
		}
		return "", "", err
	}

	targetParentPath, _, err := splitParentName(targetResolved.InnerPath)
	if err != nil {
		return "", "", err
	}
	_, targetParentPhysicalPath, err := resolvePhysicalPath(targetResolved.Source, targetParentPath)
	if err != nil {
		return "", "", err
	}
	targetParentInfo, err := os.Stat(targetParentPhysicalPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", ErrPathInvalid
		}
		return "", "", err
	}
	if !targetParentInfo.IsDir() {
		return "", "", ErrPathInvalid
	}
	if !s.canWriteLocalDir(targetParentPhysicalPath) {
		return "", "", ErrSourceReadOnly
	}

	_, targetPhysicalPath, err := resolvePhysicalPath(targetResolved.Source, targetResolved.InnerPath)
	if err != nil {
		return "", "", err
	}
	if _, err := os.Stat(targetPhysicalPath); err == nil {
		return "", "", ErrNameConflict
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}

	if sourceInfo.IsDir() {
		if err := copyDirectory(sourcePhysicalPath, targetPhysicalPath); err != nil {
			return "", "", normalizeLocalWriteError(err)
		}
	} else {
		if err := copyFile(sourcePhysicalPath, targetPhysicalPath); err != nil {
			return "", "", normalizeLocalWriteError(err)
		}
	}

	return sourceResolved.VirtualPath, targetResolved.VirtualPath, nil
}

func (s *VFSService) removeLocalResolvedPath(resolvedPath ResolvedPath) error {
	if resolvedPath.Source == nil || resolvedPath.Source.DriverType != "local" {
		return ErrSourceDriverUnsupported
	}

	_, physicalPath, err := resolvePhysicalPath(resolvedPath.Source, resolvedPath.InnerPath)
	if err != nil {
		return err
	}
	parentPhysicalPath := filepath.Dir(physicalPath)
	if parentPhysicalPath == "." {
		parentPhysicalPath = physicalPath
	}
	if !s.canWriteLocalDir(parentPhysicalPath) {
		return ErrSourceReadOnly
	}
	if err := os.RemoveAll(physicalPath); err != nil {
		return normalizeLocalWriteError(err)
	}
	return nil
}

func (s *VFSService) listMountedDirectory(ctx context.Context, currentPath string, resolved ResolvedPath) ([]appdto.VFSItem, error) {
	if resolved.Source == nil {
		return nil, ErrFileNotFound
	}

	switch resolved.Source.DriverType {
	case "local":
		_, physicalPath, err := resolvePhysicalPath(resolved.Source, resolved.InnerPath)
		if err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(physicalPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, ErrFileNotFound
			}
			return nil, err
		}

		parentWritable := s.canWriteLocalDir(physicalPath)
		items := make([]appdto.VFSItem, 0, len(entries))
		for _, entry := range entries {
			itemPath := joinVirtualPath(currentPath, entry.Name())
			if isHiddenVirtualPath(itemPath) {
				continue
			}
			info, infoErr := entry.Info()
			if infoErr != nil {
				return nil, infoErr
			}
			item := buildVFSItemFromLocal(resolved.Source.ID, itemPath, info)
			item.CanDelete = item.CanDelete && parentWritable
			items = append(items, item)
		}
		return items, nil
	default:
		driver, exists := s.fileDrivers[resolved.Source.DriverType]
		if !exists {
			return nil, ErrSourceDriverUnsupported
		}
		entries, err := driver.List(ctx, resolved.Source, resolved.InnerPath)
		if err != nil {
			switch {
			case errors.Is(err, os.ErrNotExist):
				return nil, ErrFileNotFound
			case errors.Is(err, fs.ErrInvalid), errors.Is(err, os.ErrInvalid):
				return nil, ErrFileIsDirectory
			}
			return nil, err
		}

		capabilities, err := s.driverCapabilities(ctx, resolved.Source)
		if err != nil {
			return nil, err
		}
		items := make([]appdto.VFSItem, 0, len(entries))
		for _, entry := range entries {
			entry.Path = joinVirtualPath(currentPath, entry.Name)
			if isHiddenVirtualPath(entry.Path) {
				continue
			}
			item := buildStorageEntryItem(resolved.Source.ID, entry)
			applyFileItemCapabilities(&item, capabilities)
			items = append(items, buildVFSItemFromFileItem(item, false, false))
		}
		return items, nil
	}
}

func (s *VFSService) driverCapabilities(ctx context.Context, source *entity.StorageSource) (*StorageCapabilities, error) {
	if s == nil || source == nil || s.capabilityProviders == nil {
		return nil, nil
	}
	provider, exists := s.capabilityProviders[source.DriverType]
	if !exists || provider == nil {
		return nil, nil
	}
	capabilities, err := provider.Capabilities(ctx, source)
	if err != nil {
		return nil, err
	}
	return &capabilities, nil
}

func (s *VFSService) requireFileOperator() vfsFileOperator {
	if s.fileOp == nil {
		return unsupportedVFSFileOperator{}
	}
	return s.fileOp
}

func (s *VFSService) filterReadableVFSItems(ctx context.Context, sourceID uint, items []appdto.VFSItem) ([]appdto.VFSItem, error) {
	if s.aclAuthorizer == nil {
		return items, nil
	}
	return s.aclAuthorizer.FilterVFSItems(ctx, sourceID, items)
}

func (s *VFSService) filterVisibleMounts(ctx context.Context, mounts []MountEntry) ([]MountEntry, error) {
	if s.aclAuthorizer == nil {
		return mounts, nil
	}
	filtered := make([]MountEntry, 0, len(mounts))
	for _, mount := range mounts {
		if mount.Source == nil {
			continue
		}
		visible, err := s.aclAuthorizer.CanSeeSource(ctx, mount.Source.ID)
		if err != nil {
			return nil, err
		}
		if visible {
			filtered = append(filtered, mount)
		}
	}
	return filtered, nil
}

func (s *VFSService) ensureMetadataRoot(ctx context.Context) error {
	if s.metadataReader == nil {
		return ErrSourceDriverUnsupported
	}
	if _, err := s.metadataReader.ResolveNode(ctx, "/"); err == nil {
		return nil
	} else if !errors.Is(err, ErrFileNotFound) {
		return err
	}
	_, err := s.metadataReader.EnsureRoot(ctx)
	return err
}

func (s *VFSService) refreshMetadataPathBestEffort(ctx context.Context, node *entity.VFSNode) error {
	if s.metadataRefresh == nil || node == nil || node.SourceID == nil {
		return nil
	}
	if _, err := s.metadataRefresh.RefreshPath(ctx, node.Path); err != nil {
		return err
	}
	return nil
}

func (s *VFSService) filterMetadataVFSItems(ctx context.Context, items []appdto.VFSItem) ([]appdto.VFSItem, error) {
	mounts, err := s.registry.ListEnabledMounts(ctx)
	if err != nil {
		return nil, err
	}
	visibleMounts, err := s.filterVisibleMounts(ctx, mounts)
	if err != nil {
		return nil, err
	}

	filtered := make([]appdto.VFSItem, 0, len(items))
	for _, item := range items {
		if item.SourceID == nil {
			if metadataVirtualItemVisible(item, visibleMounts, s.aclAuthorizer == nil) {
				filtered = append(filtered, item)
			}
			continue
		}

		visibleSource, err := s.metadataItemSourceVisible(ctx, *item.SourceID)
		if err != nil {
			return nil, err
		}
		if !visibleSource {
			continue
		}
		item, ok, err := s.applyMetadataItemCapabilities(ctx, item, visibleMounts)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if s.aclAuthorizer != nil {
			allowed, err := s.aclAuthorizer.FilterVFSItems(ctx, *item.SourceID, []appdto.VFSItem{item})
			if err != nil {
				return nil, err
			}
			if len(allowed) == 0 {
				continue
			}
			item = allowed[0]
		}
		filtered = append(filtered, item)
	}
	return filtered, nil
}

func (s *VFSService) metadataItemSourceVisible(ctx context.Context, sourceID uint) (bool, error) {
	if s.aclAuthorizer == nil {
		return true, nil
	}
	return s.aclAuthorizer.CanSeeSource(ctx, sourceID)
}

func (s *VFSService) applyMetadataItemCapabilities(ctx context.Context, item appdto.VFSItem, visibleMounts []MountEntry) (appdto.VFSItem, bool, error) {
	if item.SourceID == nil {
		return item, true, nil
	}
	resolved, err := resolveVirtualPathByLongestPrefix(item.Path, visibleMounts)
	if err != nil {
		return item, false, err
	}
	if !resolved.IsRealMount || resolved.Source == nil || resolved.Source.ID != *item.SourceID {
		return item, false, nil
	}

	if resolved.Source.DriverType == "local" && item.CanDelete {
		parentInnerPath := path.Dir(resolved.InnerPath)
		if parentInnerPath == "." {
			parentInnerPath = "/"
		}
		_, parentPhysicalPath, err := resolvePhysicalPath(resolved.Source, parentInnerPath)
		if err != nil {
			return item, false, err
		}
		item.CanDelete = item.CanDelete && s.canWriteLocalDir(parentPhysicalPath)
	}

	capabilities, err := s.driverCapabilities(ctx, resolved.Source)
	if err != nil {
		return item, false, err
	}
	if capabilities != nil {
		item.CanDownload = item.CanDownload && capabilities.CanDownload
		item.CanDelete = item.CanDelete && capabilities.CanDelete
	}
	return item, true, nil
}

func metadataVirtualItemVisible(item appdto.VFSItem, visibleMounts []MountEntry, bypassACL bool) bool {
	if bypassACL {
		return true
	}
	if item.SourceID != nil {
		return true
	}
	if !item.IsVirtual || item.EntryKind != string(VirtualEntryKindDirectory) {
		return false
	}
	for _, mount := range visibleMounts {
		if mount.MountPath != item.Path && isSubPath(item.Path, mount.MountPath) {
			return true
		}
	}
	return false
}

func (s *VFSService) canWriteLocalDir(dir string) bool {
	if s.localDirWritable == nil {
		return true
	}
	return s.localDirWritable(dir)
}

func normalizeVFSWriteError(err error) error {
	switch {
	case errors.Is(err, ErrFileAlreadyExists),
		errors.Is(err, ErrFileMoveConflict),
		errors.Is(err, ErrFileCopyConflict):
		return ErrNameConflict
	case errors.Is(err, ErrSourceReadOnly):
		return ErrSourceReadOnly
	default:
		return normalizeLocalWriteError(err)
	}
}

func rewriteFileItemToVFSItem(mountPath string, fileItem appdto.FileItem) appdto.VFSItem {
	fileItem.Path = mergeMountAndInnerPath(mountPath, fileItem.Path)
	fileItem.ParentPath = mergeMountAndInnerPath(mountPath, fileItem.ParentPath)
	return buildVFSItemFromFileItem(fileItem, false, false)
}
