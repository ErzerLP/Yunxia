package service

import (
	"context"
	"errors"
	"os"
	"sort"

	appdto "yunxia/internal/application/dto"
)

// VFSService 提供统一虚拟目录树的路径解析能力。
type VFSService struct {
	registry    *MountRegistry
	fileDrivers map[string]FileDriver
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
