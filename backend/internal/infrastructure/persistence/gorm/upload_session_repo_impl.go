package gorm

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

// UploadSessionRepository 提供上传会话仓储实现。
type UploadSessionRepository struct {
	db *gorm.DB
}

// NewUploadSessionRepository 创建上传会话仓储。
func NewUploadSessionRepository(db *gorm.DB) *UploadSessionRepository {
	return &UploadSessionRepository{db: db}
}

// Create 创建上传会话。
func (r *UploadSessionRepository) Create(ctx context.Context, session *entity.UploadSession) error {
	model, err := uploadSessionModelFromEntity(session)
	if err != nil {
		return err
	}
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}

	*session = *uploadSessionEntityFromModel(model)
	return nil
}

// Update 更新上传会话。
func (r *UploadSessionRepository) Update(ctx context.Context, session *entity.UploadSession) error {
	model, err := uploadSessionModelFromEntity(session)
	if err != nil {
		return err
	}
	result := r.db.WithContext(ctx).Model(&UploadSessionModel{}).Where("upload_id = ?", session.UploadID).Updates(model)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}

	return nil
}

// Delete 删除上传会话。
func (r *UploadSessionRepository) Delete(ctx context.Context, uploadID string) error {
	result := r.db.WithContext(ctx).Delete(&UploadSessionModel{}, "upload_id = ?", uploadID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}

	return nil
}

// FindByID 按 upload_id 查询会话。
func (r *UploadSessionRepository) FindByID(ctx context.Context, uploadID string) (*entity.UploadSession, error) {
	var model UploadSessionModel
	if err := r.db.WithContext(ctx).Where("upload_id = ?", uploadID).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainrepo.ErrNotFound
		}
		return nil, err
	}

	return uploadSessionEntityFromModel(&model), nil
}

// FindActiveByIdentity 查询未过期的同目标上传会话。
func (r *UploadSessionRepository) FindActiveByIdentity(ctx context.Context, userID, sourceID uint, path, filename string, fileSize int64, fileHash string) (*entity.UploadSession, error) {
	var model UploadSessionModel
	if err := r.db.WithContext(ctx).
		Where("user_id = ? AND source_id = ? AND path = ? AND filename = ? AND file_size = ? AND file_hash = ? AND status IN ? AND expires_at > ?",
			userID, sourceID, path, filename, fileSize, fileHash, []string{"pending", "uploading"}, time.Now()).
		Order("updated_at desc").
		First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainrepo.ErrNotFound
		}
		return nil, err
	}

	return uploadSessionEntityFromModel(&model), nil
}

// ListByUser 查询用户上传会话。
func (r *UploadSessionRepository) ListByUser(ctx context.Context, userID uint, sourceID *uint, status string) ([]*entity.UploadSession, error) {
	query := r.db.WithContext(ctx).Model(&UploadSessionModel{}).Where("user_id = ? AND expires_at > ?", userID, time.Now())
	if sourceID != nil {
		query = query.Where("source_id = ?", *sourceID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var models []UploadSessionModel
	if err := query.Order("updated_at desc").Find(&models).Error; err != nil {
		return nil, err
	}

	items := make([]*entity.UploadSession, 0, len(models))
	for i := range models {
		items = append(items, uploadSessionEntityFromModel(&models[i]))
	}

	return items, nil
}

func uploadSessionModelFromEntity(session *entity.UploadSession) (*UploadSessionModel, error) {
	data, err := json.Marshal(session.UploadedChunks)
	if err != nil {
		return nil, err
	}

	return &UploadSessionModel{
		UploadID:           session.UploadID,
		UserID:             session.UserID,
		SourceID:           session.SourceID,
		Path:               session.Path,
		Filename:           session.Filename,
		FileSize:           session.FileSize,
		FileHash:           session.FileHash,
		ChunkSize:          session.ChunkSize,
		TotalChunks:        session.TotalChunks,
		UploadedChunksJSON: string(data),
		StorageDataJSON:    session.StorageDataJSON,
		Status:             session.Status,
		IsFastUpload:       session.IsFastUpload,
		ExpiresAt:          session.ExpiresAt,
		CreatedAt:          session.CreatedAt,
		UpdatedAt:          session.UpdatedAt,
	}, nil
}

func uploadSessionEntityFromModel(model *UploadSessionModel) *entity.UploadSession {
	var uploaded []int
	_ = json.Unmarshal([]byte(model.UploadedChunksJSON), &uploaded)

	return &entity.UploadSession{
		UploadID:        model.UploadID,
		UserID:          model.UserID,
		SourceID:        model.SourceID,
		Path:            model.Path,
		Filename:        model.Filename,
		FileSize:        model.FileSize,
		FileHash:        model.FileHash,
		ChunkSize:       model.ChunkSize,
		TotalChunks:     model.TotalChunks,
		UploadedChunks:  uploaded,
		StorageDataJSON: model.StorageDataJSON,
		Status:          model.Status,
		IsFastUpload:    model.IsFastUpload,
		ExpiresAt:       model.ExpiresAt,
		CreatedAt:       model.CreatedAt,
		UpdatedAt:       model.UpdatedAt,
	}
}
