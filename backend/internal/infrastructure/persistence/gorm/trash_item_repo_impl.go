package gorm

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

// TrashItemRepository 提供回收站元数据仓储实现。
type TrashItemRepository struct {
	db *gorm.DB
}

// NewTrashItemRepository 创建回收站元数据仓储。
func NewTrashItemRepository(db *gorm.DB) *TrashItemRepository {
	return &TrashItemRepository{db: db}
}

// Create 创建回收站记录。
func (r *TrashItemRepository) Create(ctx context.Context, item *entity.TrashItem) error {
	model := trashItemModelFromEntity(item)
	if err := dbFor(ctx, r.db).Create(model).Error; err != nil {
		return normalizeGormError(err)
	}
	*item = *trashItemEntityFromModel(model)
	return nil
}

// FindByID 按 ID 查询回收站记录。
func (r *TrashItemRepository) FindByID(ctx context.Context, id uint) (*entity.TrashItem, error) {
	var model TrashItemModel
	if err := dbFor(ctx, r.db).First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainrepo.ErrNotFound
		}
		return nil, normalizeGormError(err)
	}
	return trashItemEntityFromModel(&model), nil
}

// ListBySourceID 返回指定 source 的回收站记录。
func (r *TrashItemRepository) ListBySourceID(ctx context.Context, sourceID uint) ([]*entity.TrashItem, error) {
	var models []TrashItemModel
	if err := dbFor(ctx, r.db).
		Where("source_id = ?", sourceID).
		Order("deleted_at desc, id desc").
		Find(&models).Error; err != nil {
		return nil, normalizeGormError(err)
	}

	items := make([]*entity.TrashItem, 0, len(models))
	for index := range models {
		items = append(items, trashItemEntityFromModel(&models[index]))
	}
	return items, nil
}

// Delete 删除回收站记录。
func (r *TrashItemRepository) Delete(ctx context.Context, id uint) error {
	result := dbFor(ctx, r.db).Delete(&TrashItemModel{}, id)
	if result.Error != nil {
		return normalizeGormError(result.Error)
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}
	return nil
}

func trashItemModelFromEntity(item *entity.TrashItem) *TrashItemModel {
	return &TrashItemModel{
		ID:                  item.ID,
		SourceID:            item.SourceID,
		OriginalPath:        item.OriginalPath,
		OriginalVirtualPath: item.OriginalVirtualPath,
		TrashPath:           item.TrashPath,
		Name:                item.Name,
		IsDir:               item.IsDir,
		Size:                item.Size,
		DeletedAt:           item.DeletedAt,
		ExpiresAt:           item.ExpiresAt,
		CreatedAt:           item.CreatedAt,
		UpdatedAt:           item.UpdatedAt,
	}
}

func trashItemEntityFromModel(model *TrashItemModel) *entity.TrashItem {
	return &entity.TrashItem{
		ID:                  model.ID,
		SourceID:            model.SourceID,
		OriginalPath:        model.OriginalPath,
		OriginalVirtualPath: model.OriginalVirtualPath,
		TrashPath:           model.TrashPath,
		Name:                model.Name,
		IsDir:               model.IsDir,
		Size:                model.Size,
		DeletedAt:           model.DeletedAt,
		ExpiresAt:           model.ExpiresAt,
		CreatedAt:           model.CreatedAt,
		UpdatedAt:           model.UpdatedAt,
	}
}
