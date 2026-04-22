package gorm

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

// RefreshTokenRepository 提供刷新令牌仓储实现。
type RefreshTokenRepository struct {
	db *gorm.DB
}

// NewRefreshTokenRepository 创建刷新令牌仓储。
func NewRefreshTokenRepository(db *gorm.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

// Create 创建刷新令牌记录。
func (r *RefreshTokenRepository) Create(ctx context.Context, token *entity.RefreshToken) error {
	model := refreshTokenModelFromEntity(token)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}

	*token = *refreshTokenEntityFromModel(model)
	return nil
}

// FindByTokenHash 按 token hash 查询刷新令牌。
func (r *RefreshTokenRepository) FindByTokenHash(ctx context.Context, tokenHash string) (*entity.RefreshToken, error) {
	var model RefreshTokenModel
	if err := r.db.WithContext(ctx).Where("token_hash = ?", tokenHash).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainrepo.ErrNotFound
		}
		return nil, err
	}

	return refreshTokenEntityFromModel(&model), nil
}

// RevokeByTokenHash 撤销刷新令牌。
func (r *RefreshTokenRepository) RevokeByTokenHash(ctx context.Context, tokenHash string) error {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&RefreshTokenModel{}).Where("token_hash = ?", tokenHash).Update("revoked_at", &now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}

	return nil
}

func refreshTokenModelFromEntity(token *entity.RefreshToken) *RefreshTokenModel {
	return &RefreshTokenModel{
		ID:        token.ID,
		UserID:    token.UserID,
		TokenHash: token.TokenHash,
		ExpiresAt: token.ExpiresAt,
		RevokedAt: token.RevokedAt,
		CreatedAt: token.CreatedAt,
		UpdatedAt: token.UpdatedAt,
	}
}

func refreshTokenEntityFromModel(model *RefreshTokenModel) *entity.RefreshToken {
	return &entity.RefreshToken{
		ID:        model.ID,
		UserID:    model.UserID,
		TokenHash: model.TokenHash,
		ExpiresAt: model.ExpiresAt,
		RevokedAt: model.RevokedAt,
		CreatedAt: model.CreatedAt,
		UpdatedAt: model.UpdatedAt,
	}
}
