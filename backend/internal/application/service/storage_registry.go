package service

import "sort"

// DriverBundle 聚合一个 storage driver 在各服务层可注册的能力。
type DriverBundle struct {
	Type        string
	DisplayName string

	Config         SourceConfigCodec
	Probe          SourceDriverProbe
	File           FileDriver
	Indexer        RemoteIndexer
	Upload         UploadDriver
	Import         ImportDriver
	NativeDownload NativeDownloadDriver
	Capacity       CapacityDriver
	Capabilities   CapabilityProvider

	// RecursiveStatsFallback 仅用于已确认可安全递归统计的 driver（当前为 S3 兼容）。
	RecursiveStatsFallback bool
}

// StorageDriverRegistry 是应用层统一的 storage driver 能力注册表。
type StorageDriverRegistry struct {
	bundles map[string]DriverBundle
}

// NewStorageDriverRegistry 创建 storage driver 注册表。
func NewStorageDriverRegistry(bundles ...DriverBundle) *StorageDriverRegistry {
	registry := &StorageDriverRegistry{bundles: make(map[string]DriverBundle)}
	for _, bundle := range bundles {
		registry.Register(bundle)
	}
	return registry
}

// Register 注册或替换一个 driver bundle。空 driver_type 会被忽略。
func (r *StorageDriverRegistry) Register(bundle DriverBundle) {
	if r == nil || bundle.Type == "" {
		return
	}
	if r.bundles == nil {
		r.bundles = make(map[string]DriverBundle)
	}
	r.bundles[bundle.Type] = bundle
}

// Bundles 返回按 driver_type 排序后的 bundle 快照，保证装配顺序稳定。
func (r *StorageDriverRegistry) Bundles() []DriverBundle {
	if r == nil {
		return nil
	}
	items := make([]DriverBundle, 0, len(r.bundles))
	for _, bundle := range r.bundles {
		items = append(items, bundle)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Type < items[j].Type
	})
	return items
}

// Bundle 查找指定 driver_type 的 bundle。
func (r *StorageDriverRegistry) Bundle(driverType string) (DriverBundle, bool) {
	if r == nil {
		return DriverBundle{}, false
	}
	bundle, exists := r.bundles[driverType]
	return bundle, exists
}

// SourceServiceOptions 生成 SourceService 所需的 driver 注册选项。
func (r *StorageDriverRegistry) SourceServiceOptions() []SourceServiceOption {
	options := []SourceServiceOption{}
	for _, bundle := range r.Bundles() {
		if bundle.Config != nil {
			options = append(options, WithSourceConfigCodec(bundle.Config))
		}
		if bundle.Probe != nil {
			options = append(options, WithSourceDriverProbe(bundle.Type, bundle.Probe))
		}
	}
	return options
}

// FileServiceOptions 生成 FileService 所需的 driver 注册选项。
func (r *StorageDriverRegistry) FileServiceOptions() []FileServiceOption {
	options := []FileServiceOption{}
	for _, bundle := range r.Bundles() {
		if bundle.File != nil {
			options = append(options, WithFileDriver(bundle.Type, bundle.File))
		}
		if bundle.Capabilities != nil {
			options = append(options, WithFileCapabilityProvider(bundle.Type, bundle.Capabilities))
		}
	}
	return options
}

// TrashServiceOptions 生成 TrashService 所需的 driver 注册选项。
func (r *StorageDriverRegistry) TrashServiceOptions() []TrashServiceOption {
	options := []TrashServiceOption{}
	for _, bundle := range r.Bundles() {
		if bundle.File != nil {
			options = append(options, WithTrashFileDriver(bundle.Type, bundle.File))
		}
	}
	return options
}

// VFSServiceOptions 生成 VFSService 所需的 driver 注册选项。
func (r *StorageDriverRegistry) VFSServiceOptions() []VFSServiceOption {
	options := []VFSServiceOption{}
	for _, bundle := range r.Bundles() {
		if bundle.File != nil {
			options = append(options, WithVFSFileDriver(bundle.Type, bundle.File))
		}
		if bundle.Capabilities != nil {
			options = append(options, WithVFSCapabilityProvider(bundle.Type, bundle.Capabilities))
		}
	}
	return options
}

// MetadataVFSSyncServiceOptions 生成 MetadataVFSSyncService 所需的 driver/indexer 注册选项。
func (r *StorageDriverRegistry) MetadataVFSSyncServiceOptions() []MetadataVFSSyncServiceOption {
	options := []MetadataVFSSyncServiceOption{}
	for _, bundle := range r.Bundles() {
		if bundle.Indexer != nil {
			options = append(options, WithMetadataVFSSyncIndexer(bundle.Type, bundle.Indexer))
		} else if bundle.File != nil {
			options = append(options, WithMetadataVFSSyncFileDriver(bundle.Type, bundle.File))
		}
	}
	return options
}

// UploadServiceOptions 生成 UploadService 所需的 driver 注册选项。
func (r *StorageDriverRegistry) UploadServiceOptions() []UploadServiceOption {
	options := []UploadServiceOption{}
	for _, bundle := range r.Bundles() {
		if bundle.Upload != nil {
			options = append(options, WithUploadDriver(bundle.Type, bundle.Upload))
		}
		if bundle.Import != nil {
			options = append(options, WithUploadImportDriver(bundle.Type, bundle.Import))
		}
	}
	return options
}

// TaskServiceOptions 生成 TaskService 所需的 driver 注册选项。
func (r *StorageDriverRegistry) TaskServiceOptions() []TaskServiceOption {
	options := []TaskServiceOption{}
	for _, bundle := range r.Bundles() {
		if bundle.Import != nil {
			options = append(options, WithTaskImportDriver(bundle.Type, bundle.Import))
		}
		if bundle.NativeDownload != nil {
			options = append(options, WithTaskNativeDownloadDriver(bundle.Type, bundle.NativeDownload))
		}
	}
	return options
}

// ShareServiceOptions 生成 ShareService 所需的 driver 注册选项。
func (r *StorageDriverRegistry) ShareServiceOptions() []ShareServiceOption {
	options := []ShareServiceOption{}
	for _, bundle := range r.Bundles() {
		if bundle.File != nil {
			options = append(options, WithShareFileDriver(bundle.Type, bundle.File))
		}
	}
	return options
}

// SystemServiceOptions 生成 SystemService 所需的 driver 注册选项。
func (r *StorageDriverRegistry) SystemServiceOptions() []SystemServiceOption {
	options := []SystemServiceOption{}
	for _, bundle := range r.Bundles() {
		if bundle.Capacity != nil {
			options = append(options, WithSystemStatsCapacityDriver(bundle.Type, bundle.Capacity))
		}
		if bundle.File != nil && bundle.RecursiveStatsFallback {
			options = append(options, WithSystemStatsFileDriver(bundle.Type, bundle.File))
		}
	}
	return options
}
