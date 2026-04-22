package repository

import (
	"context"

	"yunxia/internal/domain/entity"
)

// SourceRepository 定义存储源仓储能力。
type SourceRepository interface {
	Create(ctx context.Context, source *entity.StorageSource) error
	Update(ctx context.Context, source *entity.StorageSource) error
	Delete(ctx context.Context, id uint) error
	FindByID(ctx context.Context, id uint) (*entity.StorageSource, error)
	ListAll(ctx context.Context) ([]*entity.StorageSource, error)
	ListEnabled(ctx context.Context) ([]*entity.StorageSource, error)
	FindByName(ctx context.Context, name string) (*entity.StorageSource, error)
	Count(ctx context.Context) (int64, error)
}
