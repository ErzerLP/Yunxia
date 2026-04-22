package service

import (
	"context"
	"sort"
	"strings"

	"yunxia/internal/domain/entity"
)

type mountRegistrySourceRepository interface {
	ListAll(ctx context.Context) ([]*entity.StorageSource, error)
	ListEnabled(ctx context.Context) ([]*entity.StorageSource, error)
}

// MountRegistry 负责加载和查询挂载信息。
type MountRegistry struct {
	sourceRepo mountRegistrySourceRepository
}

// NewMountRegistry 创建挂载注册表。
func NewMountRegistry(sourceRepo mountRegistrySourceRepository) *MountRegistry {
	return &MountRegistry{sourceRepo: sourceRepo}
}

// ListAllMounts 返回全部 source 的挂载信息。
func (r *MountRegistry) ListAllMounts(ctx context.Context) ([]MountEntry, error) {
	sources, err := r.sourceRepo.ListAll(ctx)
	if err != nil {
		return nil, err
	}

	return buildMountEntries(sources)
}

// ListEnabledMounts 返回启用 source 的挂载信息。
func (r *MountRegistry) ListEnabledMounts(ctx context.Context) ([]MountEntry, error) {
	sources, err := r.sourceRepo.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}

	return buildMountEntries(sources)
}

// HasMountPathConflict 检查挂载路径是否与现有 source 冲突。
func (r *MountRegistry) HasMountPathConflict(ctx context.Context, mountPath string, excludeID uint) (bool, error) {
	normalizedMountPath, err := normalizeMountPath(mountPath)
	if err != nil {
		return false, err
	}

	mounts, err := r.ListAllMounts(ctx)
	if err != nil {
		return false, err
	}

	for _, mount := range mounts {
		if mount.Source != nil && mount.Source.ID == excludeID {
			continue
		}
		if mount.MountPath == normalizedMountPath {
			return true, nil
		}
	}

	return false, nil
}

// ProjectVirtualChildren 返回 prefix 下应投影出的直接虚拟子目录名。
func (r *MountRegistry) ProjectVirtualChildren(ctx context.Context, prefix string) ([]string, error) {
	projected, err := r.ProjectVirtualDirs(ctx, prefix)
	if err != nil {
		return nil, err
	}

	items := make([]string, 0, len(projected))
	for _, item := range projected {
		items = append(items, item.Name)
	}
	return items, nil
}

// ProjectVirtualDirs 返回 prefix 下应投影出的直接虚拟目录节点。
func (r *MountRegistry) ProjectVirtualDirs(ctx context.Context, prefix string) ([]ProjectedVirtualDir, error) {
	normalizedPrefix, err := normalizeVirtualPath(prefix)
	if err != nil {
		return nil, err
	}

	mounts, err := r.ListEnabledMounts(ctx)
	if err != nil {
		return nil, err
	}

	children := make(map[string]ProjectedVirtualDir)
	for _, mount := range mounts {
		if mount.MountPath == normalizedPrefix {
			continue
		}
		if !isSubPath(normalizedPrefix, mount.MountPath) {
			continue
		}

		relative := strings.TrimPrefix(mount.MountPath, normalizedPrefix)
		relative = strings.TrimPrefix(relative, "/")
		if relative == "" {
			continue
		}

		name := strings.SplitN(relative, "/", 2)[0]
		if name == "" {
			continue
		}
		childPath := joinVirtualPath(normalizedPrefix, name)
		existing, exists := children[name]
		if exists && existing.IsMountPoint {
			continue
		}
		children[name] = ProjectedVirtualDir{
			Name:         name,
			Path:         childPath,
			IsMountPoint: childPath == mount.MountPath,
		}
	}

	items := make([]ProjectedVirtualDir, 0, len(children))
	for _, item := range children {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})

	return items, nil
}

func buildMountEntries(sources []*entity.StorageSource) ([]MountEntry, error) {
	items := make([]MountEntry, 0, len(sources))
	for _, source := range sources {
		mountPath := source.MountPath
		if mountPath == "" && source.WebDAVSlug != "" {
			mountPath = "/" + source.WebDAVSlug
		}
		if mountPath == "" {
			continue
		}

		normalizedMountPath, err := normalizeMountPath(mountPath)
		if err != nil {
			return nil, err
		}
		items = append(items, MountEntry{
			MountPath: normalizedMountPath,
			Source:    source,
		})
	}

	return items, nil
}
