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
	if err := dbFor(ctx, r.db).Create(model).Error; err != nil {
		return normalizeGormError(err)
	}
	*task = *taskEntityFromModel(model)
	return nil
}

// Update 更新任务。
func (r *TaskRepository) Update(ctx context.Context, task *entity.DownloadTask) error {
	model := taskModelFromEntity(task)
	result := dbFor(ctx, r.db).
		Model(&DownloadTaskModel{}).
		Where("id = ?", task.ID).
		Select("*").
		Omit("ID", "CreatedAt").
		Updates(model)
	if result.Error != nil {
		return normalizeGormError(result.Error)
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}
	return nil
}

// FindByID 按 ID 查询任务。
func (r *TaskRepository) FindByID(ctx context.Context, id uint) (*entity.DownloadTask, error) {
	var model DownloadTaskModel
	if err := dbFor(ctx, r.db).First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainrepo.ErrNotFound
		}
		return nil, normalizeGormError(err)
	}
	return taskEntityFromModel(&model), nil
}

// List 返回全部任务。
func (r *TaskRepository) List(ctx context.Context) ([]*entity.DownloadTask, error) {
	var models []DownloadTaskModel
	if err := dbFor(ctx, r.db).Order("updated_at desc").Find(&models).Error; err != nil {
		return nil, normalizeGormError(err)
	}
	items := make([]*entity.DownloadTask, 0, len(models))
	for i := range models {
		items = append(items, taskEntityFromModel(&models[i]))
	}
	return items, nil
}

func taskModelFromEntity(task *entity.DownloadTask) *DownloadTaskModel {
	return &DownloadTaskModel{
		ID:                      task.ID,
		UserID:                  task.UserID,
		Type:                    task.Type,
		DownloaderType:          normalizeDownloaderType(task.DownloaderType),
		Status:                  task.Status,
		SourceID:                task.SourceID,
		SavePath:                task.SavePath,
		TargetVFSParentNodeID:   nullableUint(task.TargetVFSParentNodeID),
		TargetVirtualParentPath: task.TargetVirtualParentPath,
		TargetFilename:          task.TargetFilename,
		SaveVirtualPath:         task.SaveVirtualPath,
		ResolvedSourceID:        nullableUint(task.ResolvedSourceID),
		ResolvedInnerSavePath:   task.ResolvedInnerSavePath,
		ResultVFSNodeID:         nullableUint(task.ResultVFSNodeID),
		StagingDir:              task.StagingDir,
		DisplayName:             task.DisplayName,
		SourceURL:               task.SourceURL,
		ExternalID:              task.ExternalID,
		Progress:                task.Progress,
		DownloadedBytes:         task.DownloadedBytes,
		TotalBytes:              task.TotalBytes,
		SpeedBytes:              task.SpeedBytes,
		ETASeconds:              task.ETASeconds,
		ErrorMessage:            task.ErrorMessage,
		FinishedAt:              task.FinishedAt,
		CreatedAt:               task.CreatedAt,
		UpdatedAt:               task.UpdatedAt,
	}
}

func taskEntityFromModel(model *DownloadTaskModel) *entity.DownloadTask {
	return &entity.DownloadTask{
		ID:                      model.ID,
		UserID:                  model.UserID,
		Type:                    model.Type,
		DownloaderType:          normalizeDownloaderType(model.DownloaderType),
		Status:                  model.Status,
		SourceID:                model.SourceID,
		SavePath:                model.SavePath,
		TargetVFSParentNodeID:   uintValue(model.TargetVFSParentNodeID),
		TargetVirtualParentPath: model.TargetVirtualParentPath,
		TargetFilename:          model.TargetFilename,
		SaveVirtualPath:         model.SaveVirtualPath,
		ResolvedSourceID:        uintValue(model.ResolvedSourceID),
		ResolvedInnerSavePath:   model.ResolvedInnerSavePath,
		ResultVFSNodeID:         uintValue(model.ResultVFSNodeID),
		StagingDir:              model.StagingDir,
		DisplayName:             model.DisplayName,
		SourceURL:               model.SourceURL,
		ExternalID:              model.ExternalID,
		Progress:                model.Progress,
		DownloadedBytes:         model.DownloadedBytes,
		TotalBytes:              model.TotalBytes,
		SpeedBytes:              model.SpeedBytes,
		ETASeconds:              model.ETASeconds,
		ErrorMessage:            model.ErrorMessage,
		FinishedAt:              model.FinishedAt,
		CreatedAt:               model.CreatedAt,
		UpdatedAt:               model.UpdatedAt,
	}
}

func normalizeDownloaderType(value string) string {
	if value == "" {
		return "aria2"
	}
	return value
}
