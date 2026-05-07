package gorm

import (
	"context"
	"time"

	gormpkg "gorm.io/gorm"
	"gorm.io/gorm/clause"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

var _ domainrepo.VFSOperationRepository = (*VFSOperationRepository)(nil)

// VFSOperationRepository 提供 VFS operation journal 的 GORM 实现。
type VFSOperationRepository struct {
	db *gormpkg.DB
}

// NewVFSOperationRepository 创建 VFS operation journal 仓储。
func NewVFSOperationRepository(db *gormpkg.DB) *VFSOperationRepository {
	return &VFSOperationRepository{db: db}
}

// Create 创建 operation journal 记录。
func (r *VFSOperationRepository) Create(ctx context.Context, operation *entity.VFSOperation) error {
	model := vfsOperationModelFromEntity(operation)
	if err := dbFor(ctx, r.db).Create(model).Error; err != nil {
		return normalizeGormError(err)
	}
	*operation = *vfsOperationEntityFromModel(model)
	return nil
}

// Update 更新 operation journal 记录。
func (r *VFSOperationRepository) Update(ctx context.Context, operation *entity.VFSOperation) error {
	model := vfsOperationModelFromEntity(operation)
	result := dbFor(ctx, r.db).
		Model(&VFSOperationModel{}).
		Where("id = ?", operation.ID).
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

// FindByID 按 ID 查询 operation journal。
func (r *VFSOperationRepository) FindByID(ctx context.Context, id uint) (*entity.VFSOperation, error) {
	var model VFSOperationModel
	if err := dbFor(ctx, r.db).First(&model, id).Error; err != nil {
		return nil, normalizeGormNotFound(err)
	}
	return vfsOperationEntityFromModel(&model), nil
}

// ListDue 列出到期且未被有效 lease 锁定的 operation。
func (r *VFSOperationRepository) ListDue(ctx context.Context, filter domainrepo.VFSOperationDueFilter) ([]*entity.VFSOperation, error) {
	query := applyVFSOperationDueFilter(dbFor(ctx, r.db).Model(&VFSOperationModel{}), filter)
	var models []VFSOperationModel
	if err := query.
		Order("next_retry_at asc NULLS FIRST, id asc").
		Find(&models).Error; err != nil {
		return nil, normalizeGormError(err)
	}
	return vfsOperationEntitiesFromModels(models), nil
}

// AcquireDue 以 PostgreSQL row lock 领取到期 operation，并写入 running lease。
func (r *VFSOperationRepository) AcquireDue(ctx context.Context, filter domainrepo.VFSOperationDueFilter, lock domainrepo.VFSOperationLock) ([]*entity.VFSOperation, error) {
	if lock.WorkerID == "" {
		lock.WorkerID = "vfs-operation-worker"
	}
	if lock.LockedUntil.IsZero() {
		lock.LockedUntil = time.Now().UTC().Add(5 * time.Minute)
	}

	var acquired []VFSOperationModel
	err := dbFor(ctx, r.db).Transaction(func(tx *gormpkg.DB) error {
		query := applyVFSOperationDueFilter(tx.Model(&VFSOperationModel{}), filter)
		var models []VFSOperationModel
		if err := query.
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Order("next_retry_at asc NULLS FIRST, id asc").
			Find(&models).Error; err != nil {
			return normalizeGormError(err)
		}
		if len(models) == 0 {
			acquired = []VFSOperationModel{}
			return nil
		}

		ids := make([]uint, 0, len(models))
		for _, model := range models {
			ids = append(ids, model.ID)
		}
		updateValues := map[string]any{
			"status":       entity.VFSOperationStatusRunning,
			"locked_by":    lock.WorkerID,
			"locked_until": lock.LockedUntil,
			"updated_at":   time.Now().UTC(),
		}
		if err := tx.Model(&VFSOperationModel{}).
			Where("id IN ?", ids).
			Updates(updateValues).Error; err != nil {
			return normalizeGormError(err)
		}
		if err := tx.Where("id IN ?", ids).
			Order("next_retry_at asc NULLS FIRST, id asc").
			Find(&acquired).Error; err != nil {
			return normalizeGormError(err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return vfsOperationEntitiesFromModels(acquired), nil
}

func applyVFSOperationDueFilter(query *gormpkg.DB, filter domainrepo.VFSOperationDueFilter) *gormpkg.DB {
	statuses := filter.Statuses
	if len(statuses) == 0 {
		statuses = []string{
			entity.VFSOperationStatusPending,
			entity.VFSOperationStatusFailed,
			entity.VFSOperationStatusRunning,
		}
	}
	query = query.Where("status IN ?", statuses)
	if filter.OperationType != "" {
		query = query.Where("operation_type = ?", filter.OperationType)
	}
	dueBefore := filter.DueBefore
	if dueBefore.IsZero() {
		dueBefore = time.Now().UTC()
	}
	query = query.Where("next_retry_at IS NULL OR next_retry_at <= ?", dueBefore)
	if !filter.IncludeLocked {
		query = query.Where("locked_until IS NULL OR locked_until <= ?", dueBefore)
	}
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	return query.Limit(limit)
}

func vfsOperationEntitiesFromModels(models []VFSOperationModel) []*entity.VFSOperation {
	items := make([]*entity.VFSOperation, 0, len(models))
	for i := range models {
		items = append(items, vfsOperationEntityFromModel(&models[i]))
	}
	return items
}

func vfsOperationModelFromEntity(operation *entity.VFSOperation) *VFSOperationModel {
	return &VFSOperationModel{
		ID:                 operation.ID,
		OperationType:      operation.OperationType,
		Status:             operation.Status,
		SourceNodeID:       operation.SourceNodeID,
		TargetParentNodeID: operation.TargetParentNodeID,
		ResultNodeID:       operation.ResultNodeID,
		SourcePathSnapshot: operation.SourcePathSnapshot,
		TargetPathSnapshot: operation.TargetPathSnapshot,
		SourceIDSnapshot:   operation.SourceIDSnapshot,
		DriverTypeSnapshot: operation.DriverTypeSnapshot,
		PayloadJSON:        jsonObject(operation.PayloadJSON),
		ErrorCode:          operation.ErrorCode,
		ErrorMessage:       operation.ErrorMessage,
		RetryCount:         operation.RetryCount,
		NextRetryAt:        operation.NextRetryAt,
		LockedBy:           operation.LockedBy,
		LockedUntil:        operation.LockedUntil,
		CreatedBy:          operation.CreatedBy,
		CreatedAt:          operation.CreatedAt,
		UpdatedAt:          operation.UpdatedAt,
	}
}

func vfsOperationEntityFromModel(model *VFSOperationModel) *entity.VFSOperation {
	return &entity.VFSOperation{
		ID:                 model.ID,
		OperationType:      model.OperationType,
		Status:             model.Status,
		SourceNodeID:       model.SourceNodeID,
		TargetParentNodeID: model.TargetParentNodeID,
		ResultNodeID:       model.ResultNodeID,
		SourcePathSnapshot: model.SourcePathSnapshot,
		TargetPathSnapshot: model.TargetPathSnapshot,
		SourceIDSnapshot:   model.SourceIDSnapshot,
		DriverTypeSnapshot: model.DriverTypeSnapshot,
		PayloadJSON:        model.PayloadJSON,
		ErrorCode:          model.ErrorCode,
		ErrorMessage:       model.ErrorMessage,
		RetryCount:         model.RetryCount,
		NextRetryAt:        model.NextRetryAt,
		LockedBy:           model.LockedBy,
		LockedUntil:        model.LockedUntil,
		CreatedBy:          model.CreatedBy,
		CreatedAt:          model.CreatedAt,
		UpdatedAt:          model.UpdatedAt,
	}
}
