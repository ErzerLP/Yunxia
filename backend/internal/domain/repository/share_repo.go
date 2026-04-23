package repository

import (
	"context"

	"yunxia/internal/domain/entity"
)

// ShareRepository 定义分享链接仓储能力。
type ShareRepository interface {
	Create(ctx context.Context, share *entity.ShareLink) error
	FindByID(ctx context.Context, id uint) (*entity.ShareLink, error)
	FindByToken(ctx context.Context, token string) (*entity.ShareLink, error)
	ListAll(ctx context.Context) ([]*entity.ShareLink, error)
	ListByUser(ctx context.Context, userID uint) ([]*entity.ShareLink, error)
	Update(ctx context.Context, share *entity.ShareLink) error
	Delete(ctx context.Context, id uint) error
}
