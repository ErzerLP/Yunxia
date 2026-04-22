package repository

import (
	"context"

	"yunxia/internal/domain/entity"
)

// UploadSessionRepository 定义上传会话仓储能力。
type UploadSessionRepository interface {
	Create(ctx context.Context, session *entity.UploadSession) error
	Update(ctx context.Context, session *entity.UploadSession) error
	Delete(ctx context.Context, uploadID string) error
	FindByID(ctx context.Context, uploadID string) (*entity.UploadSession, error)
	FindActiveByIdentity(ctx context.Context, userID, sourceID uint, path, filename string, fileSize int64, fileHash string) (*entity.UploadSession, error)
	ListByUser(ctx context.Context, userID uint, sourceID *uint, status string) ([]*entity.UploadSession, error)
}
