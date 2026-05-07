package entity

import "time"

const (
	VFSOperationTypeMkdir        = "mkdir"
	VFSOperationTypeRename       = "rename"
	VFSOperationTypeMove         = "move"
	VFSOperationTypeCopy         = "copy"
	VFSOperationTypeDelete       = "delete"
	VFSOperationTypeImport       = "import"
	VFSOperationTypeUploadCommit = "upload_commit"
	VFSOperationTypeTaskCommit   = "task_commit"
	VFSOperationTypeRefresh      = "refresh"
)

const (
	VFSOperationStatusPending      = "pending"
	VFSOperationStatusRunning      = "running"
	VFSOperationStatusSucceeded    = "succeeded"
	VFSOperationStatusFailed       = "failed"
	VFSOperationStatusCompensating = "compensating"
	VFSOperationStatusCanceled     = "canceled"
)

// VFSOperation 记录跨 metadata DB 与底层 provider 非事务写操作的可恢复账本。
type VFSOperation struct {
	ID                 uint
	OperationType      string
	Status             string
	SourceNodeID       *uint
	TargetParentNodeID *uint
	ResultNodeID       *uint
	SourcePathSnapshot string
	TargetPathSnapshot string
	SourceIDSnapshot   *uint
	DriverTypeSnapshot string
	PayloadJSON        string
	ErrorCode          string
	ErrorMessage       string
	RetryCount         int
	NextRetryAt        *time.Time
	LockedBy           string
	LockedUntil        *time.Time
	CreatedBy          *uint
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
