package gorm

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

// TaskRepository 提供下载任务仓储实现。
type TaskRepository struct {
	db *gorm.DB
}

// NewTaskRepository 创建任务仓储。
func NewTaskRepository(db *gorm.DB) *TaskRepository {
	return &TaskRepository{db: db}
}

// Create 创建任务。
func (r *TaskRepository) Create(ctx context.Context, task *entity.DownloadTask) error {
	model := taskModelFromEntity(task)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	*task = *taskEntityFromModel(model)
	return nil
}

// Update 更新任务。
func (r *TaskRepository) Update(ctx context.Context, task *entity.DownloadTask) error {
	model := taskModelFromEntity(task)
	result := r.db.WithContext(ctx).Model(&DownloadTaskModel{}).Where("id = ?", task.ID).Updates(model)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}
	return nil
}

// FindByID 按 ID 查询任务。
func (r *TaskRepository) FindByID(ctx context.Context, id uint) (*entity.DownloadTask, error) {
	var model DownloadTaskModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainrepo.ErrNotFound
		}
		return nil, err
	}
	return taskEntityFromModel(&model), nil
}

// List 返回全部任务。
func (r *TaskRepository) List(ctx context.Context) ([]*entity.DownloadTask, error) {
	var models []DownloadTaskModel
	if err := r.db.WithContext(ctx).Order("updated_at desc").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]*entity.DownloadTask, 0, len(models))
	for i := range models {
		items = append(items, taskEntityFromModel(&models[i]))
	}
	return items, nil
}

func taskModelFromEntity(task *entity.DownloadTask) *DownloadTaskModel {
	return &DownloadTaskModel{
		ID:              task.ID,
		UserID:          task.UserID,
		Type:            task.Type,
		Status:          task.Status,
		SourceID:        task.SourceID,
		SavePath:        task.SavePath,
		DisplayName:     task.DisplayName,
		SourceURL:       task.SourceURL,
		ExternalID:      task.ExternalID,
		Progress:        task.Progress,
		DownloadedBytes: task.DownloadedBytes,
		TotalBytes:      task.TotalBytes,
		SpeedBytes:      task.SpeedBytes,
		ETASeconds:      task.ETASeconds,
		ErrorMessage:    task.ErrorMessage,
		FinishedAt:      task.FinishedAt,
		CreatedAt:       task.CreatedAt,
		UpdatedAt:       task.UpdatedAt,
	}
}

func taskEntityFromModel(model *DownloadTaskModel) *entity.DownloadTask {
	return &entity.DownloadTask{
		ID:              model.ID,
		UserID:          model.UserID,
		Type:            model.Type,
		Status:          model.Status,
		SourceID:        model.SourceID,
		SavePath:        model.SavePath,
		DisplayName:     model.DisplayName,
		SourceURL:       model.SourceURL,
		ExternalID:      model.ExternalID,
		Progress:        model.Progress,
		DownloadedBytes: model.DownloadedBytes,
		TotalBytes:      model.TotalBytes,
		SpeedBytes:      model.SpeedBytes,
		ETASeconds:      model.ETASeconds,
		ErrorMessage:    model.ErrorMessage,
		FinishedAt:      model.FinishedAt,
		CreatedAt:       model.CreatedAt,
		UpdatedAt:       model.UpdatedAt,
	}
}
