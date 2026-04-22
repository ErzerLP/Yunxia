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
