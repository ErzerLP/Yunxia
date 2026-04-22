package service

import (
	domainrepo "yunxia/internal/domain/repository"
	domainstorage "yunxia/internal/domain/storage"
)

// SourceDriverProbe 是 domain 层探测接口的别名。
type SourceDriverProbe = domainstorage.SourceDriverProbe

// FileDriver 是 domain 层文件驱动接口的别名。
type FileDriver = domainstorage.FileDriver

// StorageEntry 是 domain 层文件条目的别名。
type StorageEntry = domainstorage.StorageEntry

// UploadDriver 是 domain 层上传驱动接口的别名。
type UploadDriver = domainstorage.UploadDriver

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

// WithSourceACLAuthorizer 注册 SourceService 使用的 ACL 判定器。
func WithSourceACLAuthorizer(authorizer *ACLAuthorizer) SourceServiceOption {
	return func(s *SourceService) {
		s.aclAuthorizer = authorizer
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

// WithUploadACLAuthorizer 注册 UploadService 使用的 ACL 判定器。
func WithUploadACLAuthorizer(authorizer *ACLAuthorizer) UploadServiceOption {
	return func(s *UploadService) {
		s.aclAuthorizer = authorizer
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
