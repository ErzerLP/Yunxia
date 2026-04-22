package repository

import (
	"context"

	"yunxia/internal/domain/entity"
)

// UserListFilter 定义用户列表筛选条件。
type UserListFilter struct {
	Keyword string
	Status  string
}

// UserRepository 定义用户持久化能力。
type UserRepository interface {
	Create(ctx context.Context, user *entity.User) error
	FindByID(ctx context.Context, id uint) (*entity.User, error)
	FindByUsername(ctx context.Context, username string) (*entity.User, error)
	List(ctx context.Context, filter UserListFilter) ([]*entity.User, error)
	Count(ctx context.Context) (int64, error)
	Update(ctx context.Context, user *entity.User) error
	UpdateTokenVersion(ctx context.Context, id uint, tokenVersion int) error
}
