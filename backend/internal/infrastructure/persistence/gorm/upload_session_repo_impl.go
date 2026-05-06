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
		return normalizeGormError(err)
	}
	if err := dbFor(ctx, r.db).Create(model).Error; err != nil {
		return normalizeGormError(err)
	}

	*session = *uploadSessionEntityFromModel(model)
	return nil
}

// Update 更新上传会话。
func (r *UploadSessionRepository) Update(ctx context.Context, session *entity.UploadSession) error {
	model, err := uploadSessionModelFromEntity(session)
	if err != nil {
		return normalizeGormError(err)
	}
	result := dbFor(ctx, r.db).
		Model(&UploadSessionModel{}).
		Where("upload_id = ?", session.UploadID).
		Select("*").
		Omit("UploadID", "CreatedAt").
		Updates(model)
	if result.Error != nil {
		return normalizeGormError(result.Error)
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}

	return nil
}

// Delete 删除上传会话。
func (r *UploadSessionRepository) Delete(ctx context.Context, uploadID string) error {
	result := dbFor(ctx, r.db).Delete(&UploadSessionModel{}, "upload_id = ?", uploadID)
	if result.Error != nil {
		return normalizeGormError(result.Error)
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}

	return nil
}

// FindByID 按 upload_id 查询会话。
func (r *UploadSessionRepository) FindByID(ctx context.Context, uploadID string) (*entity.UploadSession, error) {
	var model UploadSessionModel
	if err := dbFor(ctx, r.db).Where("upload_id = ?", uploadID).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainrepo.ErrNotFound
		}
		return nil, normalizeGormError(err)
	}

	return uploadSessionEntityFromModel(&model), nil
}

// FindActiveByIdentity 查询未过期的同目标上传会话。
func (r *UploadSessionRepository) FindActiveByIdentity(ctx context.Context, userID, sourceID uint, path, filename string, fileSize int64, fileHash string) (*entity.UploadSession, error) {
	var model UploadSessionModel
	if err := dbFor(ctx, r.db).
		Where("user_id = ? AND source_id = ? AND path = ? AND filename = ? AND file_size = ? AND file_hash = ? AND status IN ? AND expires_at > ?",
			userID, sourceID, path, filename, fileSize, fileHash, []string{"pending", "uploading"}, time.Now()).
		Order("updated_at desc").
		First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainrepo.ErrNotFound
		}
		return nil, normalizeGormError(err)
	}

	return uploadSessionEntityFromModel(&model), nil
}

// ListByUser 查询用户上传会话。
func (r *UploadSessionRepository) ListByUser(ctx context.Context, userID uint, sourceID *uint, status string) ([]*entity.UploadSession, error) {
	query := dbFor(ctx, r.db).Model(&UploadSessionModel{}).Where("user_id = ? AND expires_at > ?", userID, time.Now())
	if sourceID != nil {
		query = query.Where("source_id = ?", *sourceID)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}

	var models []UploadSessionModel
	if err := query.Order("updated_at desc").Find(&models).Error; err != nil {
		return nil, normalizeGormError(err)
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
		return nil, normalizeGormError(err)
	}

	return &UploadSessionModel{
		UploadID:                session.UploadID,
		UserID:                  session.UserID,
		SourceID:                session.SourceID,
		Path:                    session.Path,
		TargetVirtualParentPath: session.TargetVirtualParentPath,
		ResolvedSourceID:        nullableUint(session.ResolvedSourceID),
		ResolvedInnerParentPath: session.ResolvedInnerParentPath,
		Filename:                session.Filename,
		FileSize:                session.FileSize,
		FileHash:                session.FileHash,
		ChunkSize:               session.ChunkSize,
		TotalChunks:             session.TotalChunks,
		UploadedChunksJSON:      jsonArray(string(data)),
		StorageDataJSON:         jsonObject(session.StorageDataJSON),
		Status:                  session.Status,
		IsFastUpload:            session.IsFastUpload,
		ExpiresAt:               session.ExpiresAt,
		CreatedAt:               session.CreatedAt,
		UpdatedAt:               session.UpdatedAt,
	}, nil
}

func uploadSessionEntityFromModel(model *UploadSessionModel) *entity.UploadSession {
	var uploaded []int
	_ = json.Unmarshal([]byte(model.UploadedChunksJSON), &uploaded)

	return &entity.UploadSession{
		UploadID:                model.UploadID,
		UserID:                  model.UserID,
		SourceID:                model.SourceID,
		Path:                    model.Path,
		TargetVirtualParentPath: model.TargetVirtualParentPath,
		ResolvedSourceID:        uintValue(model.ResolvedSourceID),
		ResolvedInnerParentPath: model.ResolvedInnerParentPath,
		Filename:                model.Filename,
		FileSize:                model.FileSize,
		FileHash:                model.FileHash,
		ChunkSize:               model.ChunkSize,
		TotalChunks:             model.TotalChunks,
		UploadedChunks:          uploaded,
		StorageDataJSON:         model.StorageDataJSON,
		Status:                  model.Status,
		IsFastUpload:            model.IsFastUpload,
		ExpiresAt:               model.ExpiresAt,
		CreatedAt:               model.CreatedAt,
		UpdatedAt:               model.UpdatedAt,
	}
}
