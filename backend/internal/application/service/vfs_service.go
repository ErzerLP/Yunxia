package service

import (
	"context"
	"errors"
	"os"
	"path"
	"sort"
	"time"

	appdto "yunxia/internal/application/dto"
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

// VFSService 提供统一虚拟目录树的路径解析能力。
type VFSService struct {
	registry    *MountRegistry
	fileDrivers map[string]FileDriver
	fileOp      vfsFileOperator
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

// WithVFSFileOperator 注册 VFSService 使用的底层文件操作器。
func WithVFSFileOperator(fileOp vfsFileOperator) VFSServiceOption {
	return func(s *VFSService) {
		s.fileOp = fileOp
	}
}

// NewVFSService 创建 VFS 服务。
func NewVFSService(sourceRepo mountRegistrySourceRepository, options ...VFSServiceOption) *VFSService {
	service := &VFSService{
		registry:    NewMountRegistry(sourceRepo),
		fileDrivers: make(map[string]FileDriver),
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

	return s.requireFileOperator().Delete(ctx, appdto.DeleteFileRequest{
		SourceID:   resolvedPath.Source.ID,
		Path:       resolvedPath.InnerPath,
		DeleteMode: req.DeleteMode,
	})
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
	normalizedCurrentPath, err := normalizeVirtualPath(currentPath)
	if err != nil {
		return nil, err
	}

	projectedDirs, err := s.registry.ProjectVirtualDirs(ctx, normalizedCurrentPath)
	if err != nil {
		return nil, err
	}
	mounts, err := s.registry.ListEnabledMounts(ctx)
	if err != nil {
		return nil, err
	}

	merged := make(map[string]appdto.VFSItem)
	resolved, err := resolveVirtualPathByLongestPrefix(normalizedCurrentPath, mounts)
	if err != nil {
		return nil, err
	}
	if resolved.IsRealMount {
		realItems, listErr := s.listMountedDirectory(ctx, normalizedCurrentPath, resolved)
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
			return "", "", err
		}
	} else {
		if err := copyFile(sourcePhysicalPath, targetPhysicalPath); err != nil {
			return "", "", err
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
	if err := os.RemoveAll(physicalPath); err != nil {
		return err
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
			items = append(items, buildVFSItemFromLocal(resolved.Source.ID, itemPath, info))
		}
		return items, nil
	default:
		driver, exists := s.fileDrivers[resolved.Source.DriverType]
		if !exists {
			return nil, ErrSourceDriverUnsupported
		}
		entries, err := driver.List(ctx, resolved.Source, resolved.InnerPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, ErrFileNotFound
			}
			return nil, err
		}

		items := make([]appdto.VFSItem, 0, len(entries))
		for _, entry := range entries {
			entry.Path = joinVirtualPath(currentPath, entry.Name)
			if isHiddenVirtualPath(entry.Path) {
				continue
			}
			items = append(items, buildVFSItemFromStorageEntry(resolved.Source.ID, entry))
		}
		return items, nil
	}
}

func (s *VFSService) requireFileOperator() vfsFileOperator {
	if s.fileOp == nil {
		return unsupportedVFSFileOperator{}
	}
	return s.fileOp
}

func normalizeVFSWriteError(err error) error {
	switch {
	case errors.Is(err, ErrFileAlreadyExists),
		errors.Is(err, ErrFileMoveConflict),
		errors.Is(err, ErrFileCopyConflict):
		return ErrNameConflict
	default:
		return err
	}
}

func rewriteFileItemToVFSItem(mountPath string, fileItem appdto.FileItem) appdto.VFSItem {
	fileItem.Path = mergeMountAndInnerPath(mountPath, fileItem.Path)
	fileItem.ParentPath = mergeMountAndInnerPath(mountPath, fileItem.ParentPath)
	return buildVFSItemFromFileItem(fileItem, false, false)
}
