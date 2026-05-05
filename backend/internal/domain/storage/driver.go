package storage

import (
	"context"
	"time"

	"yunxia/internal/domain/entity"
)

// SourceDriverProbe 定义存储源连通性探测能力。
type SourceDriverProbe interface {
	Test(ctx context.Context, source *entity.StorageSource) error
}

// FileDriver 定义非 local 存储驱动的最小文件能力。
type FileDriver interface {
	List(ctx context.Context, source *entity.StorageSource, virtualPath string) ([]StorageEntry, error)
	SearchByName(ctx context.Context, source *entity.StorageSource, pathPrefix, keyword string) ([]StorageEntry, error)
	Stat(ctx context.Context, source *entity.StorageSource, virtualPath string) (*StorageEntry, error)
	Mkdir(ctx context.Context, source *entity.StorageSource, parentPath, name string) (*StorageEntry, error)
	Rename(ctx context.Context, source *entity.StorageSource, virtualPath, newName string) (*StorageEntry, error)
	Move(ctx context.Context, source *entity.StorageSource, virtualPath, targetPath string) error
	Copy(ctx context.Context, source *entity.StorageSource, virtualPath, targetPath string) error
	Delete(ctx context.Context, source *entity.StorageSource, virtualPath string) error
	PresignDownload(ctx context.Context, source *entity.StorageSource, virtualPath, disposition string, ttl time.Duration) (string, time.Time, error)
}

// StorageEntry 表示驱动层抽象出的文件或目录项。
type StorageEntry struct {
	Name       string
	Path       string
	IsDir      bool
	Size       int64
	ETag       string
	ModifiedAt time.Time
}

// UploadDriver 定义直传类存储驱动的上传能力。
type UploadDriver interface {
	InitMultipartUpload(ctx context.Context, source *entity.StorageSource, req MultipartUploadRequest) (*MultipartUploadPlan, error)
	CompleteMultipartUpload(ctx context.Context, source *entity.StorageSource, state MultipartUploadState, parts []CompletedUploadPart) (*StorageEntry, error)
}

// ImportDriver 定义将后端本地暂存文件导入存储源的能力。
type ImportDriver interface {
	ImportFile(ctx context.Context, source *entity.StorageSource, targetPath string, localPath string) error
}

// NativeDownloadDriver 定义 provider 原生离线下载能力。
//
// 该接口只用于目标 source 自身支持离线下载的场景。上层仍保留通用
// 下载器 staging -> ImportFile 路径，不能把 provider 原生任务作为唯一入口。
type NativeDownloadDriver interface {
	CreateNativeDownload(ctx context.Context, source *entity.StorageSource, req NativeDownloadRequest) (*NativeDownloadTask, error)
	GetNativeDownloadStatus(ctx context.Context, source *entity.StorageSource, externalID string) (*NativeDownloadStatus, error)
	CancelNativeDownload(ctx context.Context, source *entity.StorageSource, externalID string, deleteFiles bool) error
	PauseNativeDownload(ctx context.Context, source *entity.StorageSource, externalID string) error
	ResumeNativeDownload(ctx context.Context, source *entity.StorageSource, externalID string) error
}

// NativeDownloadRequest 表示 provider 原生离线下载创建请求。
type NativeDownloadRequest struct {
	URL            string
	TargetDirPath  string
	TargetFilename string
}

// NativeDownloadTask 表示 provider 原生离线下载创建结果。
type NativeDownloadTask struct {
	ExternalID      string
	DisplayName     string
	ProgressPercent *float64
}

// NativeDownloadStatus 表示 provider 原生离线下载状态。
type NativeDownloadStatus struct {
	Status          string
	CompletedBytes  int64
	TotalBytes      *int64
	DownloadSpeed   int64
	ETASeconds      *int64
	ProgressPercent *float64
	DisplayName     string
	ErrorMessage    *string
}

// CapacityInfo 表示存储源容量信息。nil 字段表示 provider 暂不提供该值。
type CapacityInfo struct {
	UsedBytes  *int64
	TotalBytes *int64
}

// CapacityDriver 定义查询存储源容量/用量的能力。
type CapacityDriver interface {
	Capacity(ctx context.Context, source *entity.StorageSource) (*CapacityInfo, error)
}

// StorageCapabilities 描述 driver 支持的存储操作。
type StorageCapabilities struct {
	CanList           bool
	CanSearch         bool
	CanDownload       bool
	CanMkdir          bool
	CanRename         bool
	CanMove           bool
	CanCopy           bool
	CanDelete         bool
	CanProviderTrash  bool
	CanImportFile     bool
	CanDirectUpload   bool
	CanServerUpload   bool
	CanNativeDownload bool
	CanCapacity       bool
}

// CapabilityProvider 定义 driver 运行时能力查询接口。
type CapabilityProvider interface {
	Capabilities(ctx context.Context, source *entity.StorageSource) (StorageCapabilities, error)
}

// MultipartUploadRequest 表示直传上传初始化参数。
type MultipartUploadRequest struct {
	VirtualPath string
	Filename    string
	ContentType string
	FileSize    int64
	PartSize    int64
	TotalParts  int
	ExpiresIn   time.Duration
}

// MultipartUploadPlan 表示驱动生成的 multipart 计划。
type MultipartUploadPlan struct {
	State            MultipartUploadState
	PartInstructions []MultipartUploadPartInstruction
}

// MultipartUploadState 表示直传完成所需的持久化状态。
type MultipartUploadState struct {
	RemoteUploadID string `json:"remote_upload_id"`
	ObjectKey      string `json:"object_key"`
	VirtualPath    string `json:"virtual_path"`
}

// MultipartUploadPartInstruction 表示单个 part 的上传说明。
type MultipartUploadPartInstruction struct {
	Index     int
	Method    string
	URL       string
	Headers   map[string]string
	ByteStart int64
	ByteEnd   int64
	ExpiresAt time.Time
}

// CompletedUploadPart 表示前端回传的已上传 part。
type CompletedUploadPart struct {
	Index int
	ETag  string
}
