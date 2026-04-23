package repository

import (
	"context"

	"yunxia/internal/domain/entity"
)

// AuditLogRepository 定义审计日志持久化能力。
type AuditLogRepository interface {
	Create(ctx context.Context, log *entity.AuditLog) error
	FindByID(ctx context.Context, id uint) (*entity.AuditLog, error)
	List(ctx context.Context, filter entity.AuditLogFilter) ([]*entity.AuditLog, int, error)
}
