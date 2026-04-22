package repository

import (
	"context"

	"yunxia/internal/domain/entity"
)

// RefreshTokenRepository 定义刷新令牌持久化能力。
type RefreshTokenRepository interface {
	Create(ctx context.Context, token *entity.RefreshToken) error
	FindByTokenHash(ctx context.Context, tokenHash string) (*entity.RefreshToken, error)
	RevokeByTokenHash(ctx context.Context, tokenHash string) error
}
