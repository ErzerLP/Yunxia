package gorm

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

// SourceRepository 提供存储源仓储实现。
type SourceRepository struct {
	db *gorm.DB
}

// NewSourceRepository 创建存储源仓储。
func NewSourceRepository(db *gorm.DB) *SourceRepository {
	return &SourceRepository{db: db}
}

// Create 创建存储源。
func (r *SourceRepository) Create(ctx context.Context, source *entity.StorageSource) error {
	model := sourceModelFromEntity(source)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}

	*source = *sourceEntityFromModel(model)
	return nil
}

// Update 更新存储源。
func (r *SourceRepository) Update(ctx context.Context, source *entity.StorageSource) error {
	model := sourceModelFromEntity(source)
	result := r.db.WithContext(ctx).
		Model(&StorageSourceModel{}).
		Where("id = ?", source.ID).
		Select("*").
		Omit("ID", "CreatedAt").
		Updates(model)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}

	return nil
}

// Delete 删除存储源。
func (r *SourceRepository) Delete(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&StorageSourceModel{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}

	return nil
}

// FindByID 按 ID 查询存储源。
func (r *SourceRepository) FindByID(ctx context.Context, id uint) (*entity.StorageSource, error) {
	var model StorageSourceModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainrepo.ErrNotFound
		}
		return nil, err
	}

	return sourceEntityFromModel(&model), nil
}

// ListAll 查询全部存储源。
func (r *SourceRepository) ListAll(ctx context.Context) ([]*entity.StorageSource, error) {
	var models []StorageSourceModel
	if err := r.db.WithContext(ctx).Order("sort_order asc, id asc").Find(&models).Error; err != nil {
		return nil, err
	}

	items := make([]*entity.StorageSource, 0, len(models))
	for i := range models {
		items = append(items, sourceEntityFromModel(&models[i]))
	}

	return items, nil
}

// ListEnabled 查询启用的存储源。
func (r *SourceRepository) ListEnabled(ctx context.Context) ([]*entity.StorageSource, error) {
	var models []StorageSourceModel
	if err := r.db.WithContext(ctx).Where("is_enabled = ?", true).Order("sort_order asc, id asc").Find(&models).Error; err != nil {
		return nil, err
	}

	items := make([]*entity.StorageSource, 0, len(models))
	for i := range models {
		items = append(items, sourceEntityFromModel(&models[i]))
	}

	return items, nil
}

// FindByName 按名称查询存储源。
func (r *SourceRepository) FindByName(ctx context.Context, name string) (*entity.StorageSource, error) {
	var model StorageSourceModel
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainrepo.ErrNotFound
		}
		return nil, err
	}

	return sourceEntityFromModel(&model), nil
}

// Count 统计存储源数量。
func (r *SourceRepository) Count(ctx context.Context) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&StorageSourceModel{}).Count(&count).Error; err != nil {
		return 0, err
	}

	return count, nil
}

func sourceModelFromEntity(source *entity.StorageSource) *StorageSourceModel {
	return &StorageSourceModel{
		ID:              source.ID,
		Name:            source.Name,
		DriverType:      source.DriverType,
		Status:          source.Status,
		IsEnabled:       source.IsEnabled,
		IsWebDAVExposed: source.IsWebDAVExposed,
		WebDAVReadOnly:  source.WebDAVReadOnly,
		WebDAVSlug:      source.WebDAVSlug,
		MountPath:       source.MountPath,
		RootPath:        source.RootPath,
		SortOrder:       source.SortOrder,
		ConfigJSON:      source.ConfigJSON,
		LastCheckedAt:   source.LastCheckedAt,
		CreatedAt:       source.CreatedAt,
		UpdatedAt:       source.UpdatedAt,
	}
}

func sourceEntityFromModel(model *StorageSourceModel) *entity.StorageSource {
	return &entity.StorageSource{
		ID:              model.ID,
		Name:            model.Name,
		DriverType:      model.DriverType,
		Status:          model.Status,
		IsEnabled:       model.IsEnabled,
		IsWebDAVExposed: model.IsWebDAVExposed,
		WebDAVReadOnly:  model.WebDAVReadOnly,
		WebDAVSlug:      model.WebDAVSlug,
		MountPath:       model.MountPath,
		RootPath:        model.RootPath,
		SortOrder:       model.SortOrder,
		ConfigJSON:      model.ConfigJSON,
		LastCheckedAt:   model.LastCheckedAt,
		CreatedAt:       model.CreatedAt,
		UpdatedAt:       model.UpdatedAt,
	}
}
