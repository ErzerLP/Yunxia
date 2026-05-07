package service

import (
	"context"

	domainrepo "yunxia/internal/domain/repository"
	domainstorage "yunxia/internal/domain/storage"
)

// SourceDriverProbe 是 domain 层探测接口的别名。
type SourceDriverProbe = domainstorage.SourceDriverProbe

// FileDriver 是 domain 层文件驱动接口的别名。
type FileDriver = domainstorage.FileDriver

// StorageEntry 是 domain 层文件条目的别名。
type StorageEntry = domainstorage.StorageEntry

// RemoteIndexer 是 domain 层远端懒索引接口的别名。
type RemoteIndexer = domainstorage.RemoteIndexer

// RemoteListRequest 是 domain 层远端列举请求的别名。
type RemoteListRequest = domainstorage.RemoteListRequest

// RemoteEntry 是 domain 层远端条目的别名。
type RemoteEntry = domainstorage.RemoteEntry

// ObjectReader 是 domain 层基于 storage object locator 的读取接口别名。
type ObjectReader = domainstorage.ObjectReader

// UploadDriver 是 domain 层上传驱动接口的别名。
type UploadDriver = domainstorage.UploadDriver

// ImportDriver 是 domain 层本地暂存文件导入接口的别名。
type ImportDriver = domainstorage.ImportDriver

// TaskImportDriver 保留旧任务导入命名兼容，底层复用通用 ImportDriver。
type TaskImportDriver = domainstorage.ImportDriver

// NativeDownloadDriver 是 domain 层 provider 原生离线下载接口的别名。
type NativeDownloadDriver = domainstorage.NativeDownloadDriver

// NativeDownloadRequest 是 provider 原生离线下载创建请求的别名。
type NativeDownloadRequest = domainstorage.NativeDownloadRequest

// NativeDownloadTask 是 provider 原生离线下载创建结果的别名。
type NativeDownloadTask = domainstorage.NativeDownloadTask

// NativeDownloadStatus 是 provider 原生离线下载状态的别名。
type NativeDownloadStatus = domainstorage.NativeDownloadStatus

// CapacityInfo 是 domain 层容量信息的别名。
type CapacityInfo = domainstorage.CapacityInfo

// CapacityDriver 是 domain 层容量查询接口的别名。
type CapacityDriver = domainstorage.CapacityDriver

// StorageCapabilities 是 domain 层 driver 能力描述的别名。
type StorageCapabilities = domainstorage.StorageCapabilities

// CapabilityProvider 是 domain 层 driver 能力查询接口的别名。
type CapabilityProvider = domainstorage.CapabilityProvider

// MultipartUploadRequest 是 domain 层直传请求的别名。
type MultipartUploadRequest = domainstorage.MultipartUploadRequest

// MultipartUploadPlan 是 domain 层直传计划的别名。
type MultipartUploadPlan = domainstorage.MultipartUploadPlan

// MultipartUploadState 是 domain 层直传状态的别名。
type MultipartUploadState = domainstorage.MultipartUploadState

// MultipartUploadPartInstruction 是 domain 层分片说明的别名。
type MultipartUploadPartInstruction = domainstorage.MultipartUploadPartInstruction

// CompletedUploadPart 是 domain 层已完成分片的别名。
type CompletedUploadPart = domainstorage.CompletedUploadPart

// SourceServiceOption 定义 SourceService 的可选配置。
type SourceServiceOption func(*SourceService)

// WithSourceDriverProbe 注册指定 driver 的探测器。
func WithSourceDriverProbe(driverType string, probe SourceDriverProbe) SourceServiceOption {
	return func(s *SourceService) {
		if driverType == "" || probe == nil {
			return
		}
		if s.driverProbes == nil {
			s.driverProbes = make(map[string]SourceDriverProbe)
		}
		s.driverProbes[driverType] = probe
	}
}

// WithSourceConfigCodec 注册指定 driver 的配置编解码器。
func WithSourceConfigCodec(codec SourceConfigCodec) SourceServiceOption {
	return func(s *SourceService) {
		if codec == nil || codec.DriverType() == "" {
			return
		}
		if s.configCodecs == nil {
			s.configCodecs = make(map[string]SourceConfigCodec)
		}
		s.configCodecs[codec.DriverType()] = codec
	}
}

// WithSourceACLAuthorizer 注册 SourceService 使用的 ACL 判定器。
func WithSourceACLAuthorizer(authorizer *ACLAuthorizer) SourceServiceOption {
	return func(s *SourceService) {
		s.aclAuthorizer = authorizer
	}
}

// WithSourceMountSyncer 注册 SourceService 的 metadata VFS mount 同步端口。
func WithSourceMountSyncer(syncer MetadataSourceMountSyncer) SourceServiceOption {
	return func(s *SourceService) {
		s.mountSyncer = syncer
	}
}

// WithSourceTransactor 注册 SourceService 使用的事务端口。
func WithSourceTransactor(transactor domainrepo.Transactor) SourceServiceOption {
	return func(s *SourceService) {
		s.transactor = transactor
	}
}

// FileServiceOption 定义 FileService 的可选配置。
type FileServiceOption func(*FileService)

// WithFileDriver 注册指定 driver 的文件驱动。
func WithFileDriver(driverType string, driver FileDriver) FileServiceOption {
	return func(s *FileService) {
		if driverType == "" || driver == nil {
			return
		}
		if s.fileDrivers == nil {
			s.fileDrivers = make(map[string]FileDriver)
		}
		s.fileDrivers[driverType] = driver
	}
}

// WithFileObjectReader 注册指定 driver 的 object locator 读取器。
func WithFileObjectReader(driverType string, reader ObjectReader) FileServiceOption {
	return func(s *FileService) {
		if driverType == "" || reader == nil {
			return
		}
		if s.objectReaders == nil {
			s.objectReaders = make(map[string]ObjectReader)
		}
		s.objectReaders[driverType] = reader
	}
}

// WithFileCapabilityProvider 注册指定 driver 的能力描述器。
func WithFileCapabilityProvider(driverType string, provider CapabilityProvider) FileServiceOption {
	return func(s *FileService) {
		if driverType == "" || provider == nil {
			return
		}
		if s.capabilityProviders == nil {
			s.capabilityProviders = make(map[string]CapabilityProvider)
		}
		s.capabilityProviders[driverType] = provider
	}
}

// WithTrashItemRepository 注册 FileService 使用的回收站元数据仓储。
func WithTrashItemRepository(repo domainrepo.TrashItemRepository) FileServiceOption {
	return func(s *FileService) {
		s.trashItemRepo = repo
	}
}

// WithFileACLAuthorizer 注册 FileService 使用的 ACL 判定器。
func WithFileACLAuthorizer(authorizer *ACLAuthorizer) FileServiceOption {
	return func(s *FileService) {
		s.aclAuthorizer = authorizer
	}
}

// WithFileMetadataVFSObjectAccess 注册 VFS node/object 读模型，用于 object-first 下载。
func WithFileMetadataVFSObjectAccess(reader metadataVFSReader, objectRepo domainrepo.StorageObjectRepository) FileServiceOption {
	return func(s *FileService) {
		s.metadataReader = reader
		s.storageObjectRepo = objectRepo
	}
}

// WithFileLocalDirWritable 覆盖本地目录可写探测能力，主要用于测试只读挂载。
func WithFileLocalDirWritable(checker func(string) bool) FileServiceOption {
	return func(s *FileService) {
		if checker != nil {
			s.localDirWritable = checker
		}
	}
}

// TrashServiceOption 定义 TrashService 的可选配置。
type TrashServiceOption func(*TrashService)

// WithTrashFileDriver 注册 TrashService 使用的文件驱动。
func WithTrashFileDriver(driverType string, driver FileDriver) TrashServiceOption {
	return func(s *TrashService) {
		if driverType == "" || driver == nil {
			return
		}
		if s.fileDrivers == nil {
			s.fileDrivers = make(map[string]FileDriver)
		}
		s.fileDrivers[driverType] = driver
	}
}

// WithTrashACLAuthorizer 注册 TrashService 使用的 ACL 判定器。
func WithTrashACLAuthorizer(authorizer *ACLAuthorizer) TrashServiceOption {
	return func(s *TrashService) {
		s.aclAuthorizer = authorizer
	}
}

// UploadServiceOption 定义 UploadService 的可选配置。
type UploadServiceOption func(*UploadService)

// WithUploadDriver 注册指定 driver 的上传驱动。
func WithUploadDriver(driverType string, driver UploadDriver) UploadServiceOption {
	return func(s *UploadService) {
		if driverType == "" || driver == nil {
			return
		}
		if s.uploadDrivers == nil {
			s.uploadDrivers = make(map[string]UploadDriver)
		}
		s.uploadDrivers[driverType] = driver
	}
}

// WithUploadImportDriver 注册后端 server_chunk 上传完成后的导入驱动。
func WithUploadImportDriver(driverType string, driver ImportDriver) UploadServiceOption {
	return func(s *UploadService) {
		if driverType == "" || driver == nil {
			return
		}
		if s.importDrivers == nil {
			s.importDrivers = make(map[string]ImportDriver)
		}
		s.importDrivers[driverType] = driver
	}
}

// WithUploadACLAuthorizer 注册 UploadService 使用的 ACL 判定器。
func WithUploadACLAuthorizer(authorizer *ACLAuthorizer) UploadServiceOption {
	return func(s *UploadService) {
		s.aclAuthorizer = authorizer
	}
}

// WithUploadVFSResolver 注册 UploadService 使用的 VFS 写入落点解析器。
func WithUploadVFSResolver(resolver interface {
	ResolveWritableTarget(ctx context.Context, virtualPath string) (ResolvedPath, error)
}) UploadServiceOption {
	return func(s *UploadService) {
		s.vfsResolver = resolver
	}
}

// WithUploadMetadataVFSCommitter 注册上传完成后的 metadata VFS 提交端口。
func WithUploadMetadataVFSCommitter(committer MetadataVFSCommitter) UploadServiceOption {
	return func(s *UploadService) {
		s.metadataCommitter = committer
	}
}

// WithUploadOperationJournal 注册上传 metadata commit 失败时的 operation journal。
func WithUploadOperationJournal(journal vfsOperationJournalRecorder) UploadServiceOption {
	return func(s *UploadService) {
		s.operationJournal = journal
	}
}

// WithUploadMetadataVFSReader 注册上传目标 metadata VFS node 快照解析器。
func WithUploadMetadataVFSReader(reader metadataVFSReader) UploadServiceOption {
	return func(s *UploadService) {
		s.metadataReader = reader
	}
}

// TaskServiceOption 定义 TaskService 的可选配置。
type TaskServiceOption func(*TaskService)

// WithTaskACLAuthorizer 注册 TaskService 使用的 ACL 判定器。
func WithTaskACLAuthorizer(authorizer *ACLAuthorizer) TaskServiceOption {
	return func(s *TaskService) {
		s.aclAuthorizer = authorizer
	}
}

// WithTaskStagingDir 设置离线下载的本地暂存根目录。
func WithTaskStagingDir(dir string) TaskServiceOption {
	return func(s *TaskService) {
		if dir == "" {
			return
		}
		s.stagingRoot = dir
	}
}

// WithTaskDownloaderStagingDir 设置指定下载器的暂存根目录。
func WithTaskDownloaderStagingDir(downloaderType string, dir string) TaskServiceOption {
	return func(s *TaskService) {
		if downloaderType == "" || dir == "" {
			return
		}
		if s.stagingRoots == nil {
			s.stagingRoots = make(map[string]string)
		}
		s.stagingRoots[downloaderType] = dir
	}
}

// WithTaskImportDriver 注册下载完成后导入远端存储源的驱动。
func WithTaskImportDriver(driverType string, driver TaskImportDriver) TaskServiceOption {
	return func(s *TaskService) {
		if driverType == "" || driver == nil {
			return
		}
		if s.importDrivers == nil {
			s.importDrivers = make(map[string]TaskImportDriver)
		}
		s.importDrivers[driverType] = driver
	}
}

// WithTaskNativeDownloadDriver 注册 provider 原生离线下载驱动。
func WithTaskNativeDownloadDriver(driverType string, driver NativeDownloadDriver) TaskServiceOption {
	return func(s *TaskService) {
		if driverType == "" || driver == nil {
			return
		}
		if s.nativeDownloadDrivers == nil {
			s.nativeDownloadDrivers = make(map[string]NativeDownloadDriver)
		}
		s.nativeDownloadDrivers[driverType] = driver
	}
}

// WithTaskDownloadRouter 注册按链接类型分发的下载器路由。
func WithTaskDownloadRouter(router *DownloaderRouter) TaskServiceOption {
	return func(s *TaskService) {
		if router != nil {
			s.downloadRouter = router
		}
	}
}

// WithTaskVFSResolver 注册 TaskService 使用的统一虚拟目录解析器。
func WithTaskVFSResolver(resolver interface {
	ResolveWritableTarget(ctx context.Context, virtualPath string) (ResolvedPath, error)
}) TaskServiceOption {
	return func(s *TaskService) {
		s.vfsResolver = resolver
	}
}

// WithTaskMetadataVFSCommitter 注册任务完成导入后的 metadata VFS 提交端口。
func WithTaskMetadataVFSCommitter(committer MetadataVFSCommitter) TaskServiceOption {
	return func(s *TaskService) {
		s.metadataCommitter = committer
	}
}

// WithTaskOperationJournal 注册任务 metadata commit 失败时的 operation journal。
func WithTaskOperationJournal(journal vfsOperationJournalRecorder) TaskServiceOption {
	return func(s *TaskService) {
		s.operationJournal = journal
	}
}

// WithTaskMetadataVFSReader 注册下载任务目标 metadata VFS node 快照解析器。
func WithTaskMetadataVFSReader(reader metadataVFSReader) TaskServiceOption {
	return func(s *TaskService) {
		s.metadataReader = reader
	}
}

// ShareServiceOption 定义 ShareService 的可选配置。
type ShareServiceOption func(*ShareService)

// WithShareFileDriver 注册分享服务使用的文件驱动。
func WithShareFileDriver(driverType string, driver FileDriver) ShareServiceOption {
	return func(s *ShareService) {
		if driverType == "" || driver == nil {
			return
		}
		if s.fileDrivers == nil {
			s.fileDrivers = make(map[string]FileDriver)
		}
		s.fileDrivers[driverType] = driver
	}
}

// WithShareACLAuthorizer 注册 ShareService 使用的 ACL 判定器。
func WithShareACLAuthorizer(authorizer *ACLAuthorizer) ShareServiceOption {
	return func(s *ShareService) {
		s.aclAuthorizer = authorizer
	}
}

// WithShareMetadataVFS 注册分享目标 VFS node 解析与懒刷新能力。
func WithShareMetadataVFS(reader metadataVFSReader, refresh metadataVFSRefreshService) ShareServiceOption {
	return func(s *ShareService) {
		s.metadataReader = reader
		s.metadataRefresh = refresh
	}
}
