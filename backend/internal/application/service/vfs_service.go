package service

import "context"

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
