package gorm

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

// SystemConfigRepository 提供系统配置仓储实现。
type SystemConfigRepository struct {
	db *gorm.DB
}

// NewSystemConfigRepository 创建系统配置仓储。
func NewSystemConfigRepository(db *gorm.DB) *SystemConfigRepository {
	return &SystemConfigRepository{db: db}
}

// Get 获取系统配置。
func (r *SystemConfigRepository) Get(ctx context.Context) (*entity.SystemConfig, error) {
	var model SystemConfigModel
	if err := r.db.WithContext(ctx).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainrepo.ErrNotFound
		}
		return nil, err
	}

	return systemConfigEntityFromModel(&model), nil
}

// Upsert 创建或更新系统配置。
func (r *SystemConfigRepository) Upsert(ctx context.Context, cfg *entity.SystemConfig) error {
	model := systemConfigModelFromEntity(cfg)
	if model.ID == 0 {
		model.ID = 1
	}

	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(model).Error; err != nil {
		return err
	}

	cfg.ID = model.ID
	cfg.CreatedAt = model.CreatedAt
	cfg.UpdatedAt = model.UpdatedAt
	return nil
}

func systemConfigModelFromEntity(cfg *entity.SystemConfig) *SystemConfigModel {
	return &SystemConfigModel{
		ID:               cfg.ID,
		SiteName:         cfg.SiteName,
		MultiUserEnabled: cfg.MultiUserEnabled,
		DefaultSourceID:  cfg.DefaultSourceID,
		MaxUploadSize:    cfg.MaxUploadSize,
		DefaultChunkSize: cfg.DefaultChunkSize,
		WebDAVEnabled:    cfg.WebDAVEnabled,
		WebDAVPrefix:     cfg.WebDAVPrefix,
		Theme:            cfg.Theme,
		Language:         cfg.Language,
		TimeZone:         cfg.TimeZone,
		CreatedAt:        cfg.CreatedAt,
		UpdatedAt:        cfg.UpdatedAt,
	}
}

func systemConfigEntityFromModel(model *SystemConfigModel) *entity.SystemConfig {
	return &entity.SystemConfig{
		ID:               model.ID,
		SiteName:         model.SiteName,
		MultiUserEnabled: model.MultiUserEnabled,
		DefaultSourceID:  model.DefaultSourceID,
		MaxUploadSize:    model.MaxUploadSize,
		DefaultChunkSize: model.DefaultChunkSize,
		WebDAVEnabled:    model.WebDAVEnabled,
		WebDAVPrefix:     model.WebDAVPrefix,
		Theme:            model.Theme,
		Language:         model.Language,
		TimeZone:         model.TimeZone,
		CreatedAt:        model.CreatedAt,
		UpdatedAt:        model.UpdatedAt,
	}
}
