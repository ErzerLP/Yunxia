package repository

import (
	"context"
	"time"

	"yunxia/internal/domain/entity"
)

// VFSOperationDueFilter 定义 operation journal 到期领取筛选条件。
// 默认包含 pending、failed 以及 lease 已过期的 running，避免 worker 崩溃后永久卡住。
type VFSOperationDueFilter struct {
	OperationType string
	Statuses      []string
	DueBefore     time.Time
	Limit         int
	IncludeLocked bool
}

// VFSOperationLock 定义领取 operation 时写入的 lease 信息。
type VFSOperationLock struct {
	WorkerID    string
	LockedUntil time.Time
}

// VFSOperationRepository 定义 VFS operation journal 持久化能力。
type VFSOperationRepository interface {
	Create(ctx context.Context, operation *entity.VFSOperation) error
	Update(ctx context.Context, operation *entity.VFSOperation) error
	FindByID(ctx context.Context, id uint) (*entity.VFSOperation, error)
	ListDue(ctx context.Context, filter VFSOperationDueFilter) ([]*entity.VFSOperation, error)
	AcquireDue(ctx context.Context, filter VFSOperationDueFilter, lock VFSOperationLock) ([]*entity.VFSOperation, error)
}
