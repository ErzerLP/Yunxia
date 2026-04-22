package repository

import (
	"context"

	"yunxia/internal/domain/entity"
)

// SystemConfigRepository 定义系统配置持久化能力。
type SystemConfigRepository interface {
	Get(ctx context.Context) (*entity.SystemConfig, error)
	Upsert(ctx context.Context, cfg *entity.SystemConfig) error
}
