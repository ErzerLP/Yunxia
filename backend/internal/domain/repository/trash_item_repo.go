package repository

import (
	"context"

	"yunxia/internal/domain/entity"
)

// TrashItemRepository 定义回收站元数据持久化能力。
type TrashItemRepository interface {
	Create(ctx context.Context, item *entity.TrashItem) error
	FindByID(ctx context.Context, id uint) (*entity.TrashItem, error)
	ListBySourceID(ctx context.Context, sourceID uint) ([]*entity.TrashItem, error)
	Delete(ctx context.Context, id uint) error
}
