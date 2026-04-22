package gorm

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

// ShareRepository 提供分享链接仓储实现。
type ShareRepository struct {
	db *gorm.DB
}

// NewShareRepository 创建分享链接仓储。
func NewShareRepository(db *gorm.DB) *ShareRepository {
	return &ShareRepository{db: db}
}

// Create 创建分享链接。
func (r *ShareRepository) Create(ctx context.Context, share *entity.ShareLink) error {
	model := shareModelFromEntity(share)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	*share = *shareEntityFromModel(model)
	return nil
}

// FindByID 按 ID 查询分享链接。
func (r *ShareRepository) FindByID(ctx context.Context, id uint) (*entity.ShareLink, error) {
	var model ShareLinkModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainrepo.ErrNotFound
		}
		return nil, err
	}
	return shareEntityFromModel(&model), nil
}

// FindByToken 按 token 查询分享链接。
func (r *ShareRepository) FindByToken(ctx context.Context, token string) (*entity.ShareLink, error) {
	var model ShareLinkModel
	if err := r.db.WithContext(ctx).Where("token = ?", token).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainrepo.ErrNotFound
		}
		return nil, err
	}
	return shareEntityFromModel(&model), nil
}

// ListByUser 返回当前用户创建的分享链接。
func (r *ShareRepository) ListByUser(ctx context.Context, userID uint) ([]*entity.ShareLink, error) {
	var models []ShareLinkModel
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("created_at desc, id desc").Find(&models).Error; err != nil {
		return nil, err
	}
	items := make([]*entity.ShareLink, 0, len(models))
	for i := range models {
		items = append(items, shareEntityFromModel(&models[i]))
	}
	return items, nil
}

// Update 更新分享链接。
func (r *ShareRepository) Update(ctx context.Context, share *entity.ShareLink) error {
	values := map[string]any{
		"user_id":       share.UserID,
		"source_id":     share.SourceID,
		"path":          share.Path,
		"name":          share.Name,
		"is_dir":        share.IsDir,
		"token":         share.Token,
		"password_hash": share.PasswordHash,
		"expires_at":    share.ExpiresAt,
		"updated_at":    share.UpdatedAt,
	}
	result := r.db.WithContext(ctx).Model(&ShareLinkModel{}).Where("id = ?", share.ID).Updates(values)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}
	return r.findInto(ctx, share.ID, share)
}

// Delete 删除分享链接。
func (r *ShareRepository) Delete(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&ShareLinkModel{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}
	return nil
}

func (r *ShareRepository) findInto(ctx context.Context, id uint, share *entity.ShareLink) error {
	model := ShareLinkModel{}
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domainrepo.ErrNotFound
		}
		return err
	}
	*share = *shareEntityFromModel(&model)
	return nil
}

func shareModelFromEntity(share *entity.ShareLink) *ShareLinkModel {
	return &ShareLinkModel{
		ID:           share.ID,
		UserID:       share.UserID,
		SourceID:     share.SourceID,
		Path:         share.Path,
		Name:         share.Name,
		IsDir:        share.IsDir,
		Token:        share.Token,
		PasswordHash: share.PasswordHash,
		ExpiresAt:    share.ExpiresAt,
		CreatedAt:    share.CreatedAt,
		UpdatedAt:    share.UpdatedAt,
	}
}

func shareEntityFromModel(model *ShareLinkModel) *entity.ShareLink {
	return &entity.ShareLink{
		ID:           model.ID,
		UserID:       model.UserID,
		SourceID:     model.SourceID,
		Path:         model.Path,
		Name:         model.Name,
		IsDir:        model.IsDir,
		Token:        model.Token,
		PasswordHash: model.PasswordHash,
		ExpiresAt:    model.ExpiresAt,
		CreatedAt:    model.CreatedAt,
		UpdatedAt:    model.UpdatedAt,
	}
}
