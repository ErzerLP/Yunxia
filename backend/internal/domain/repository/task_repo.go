package repository

import (
	"context"

	"yunxia/internal/domain/entity"
)

// TaskRepository 定义下载任务仓储能力。
type TaskRepository interface {
	Create(ctx context.Context, task *entity.DownloadTask) error
	Update(ctx context.Context, task *entity.DownloadTask) error
	FindByID(ctx context.Context, id uint) (*entity.DownloadTask, error)
	List(ctx context.Context) ([]*entity.DownloadTask, error)
}
