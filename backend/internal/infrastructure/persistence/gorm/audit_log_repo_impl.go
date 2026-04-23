package gorm

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	"yunxia/internal/domain/entity"
)

// AuditLogRepository 提供审计日志仓储实现。
type AuditLogRepository struct {
	db *gorm.DB
}

// NewAuditLogRepository 创建审计日志仓储。
func NewAuditLogRepository(db *gorm.DB) *AuditLogRepository {
	return &AuditLogRepository{db: db}
}

// Create 创建审计日志。
func (r *AuditLogRepository) Create(ctx context.Context, log *entity.AuditLog) error {
	model := auditLogModelFromEntity(log)
	if err := r.db.WithContext(ctx).Create(model).Error; err != nil {
		return err
	}
	*log = *auditLogEntityFromModel(model)
	return nil
}

// FindByID 按 ID 查询审计日志。
func (r *AuditLogRepository) FindByID(ctx context.Context, id uint) (*entity.AuditLog, error) {
	var model AuditLogModel
	if err := r.db.WithContext(ctx).First(&model, id).Error; err != nil {
		return nil, normalizeError(err)
	}
	return auditLogEntityFromModel(&model), nil
}

// List 返回过滤后的审计日志。
func (r *AuditLogRepository) List(ctx context.Context, filter entity.AuditLogFilter) ([]*entity.AuditLog, int, error) {
	query := r.db.WithContext(ctx).Model(&AuditLogModel{})

	if filter.ActorUserID != nil {
		query = query.Where("actor_user_id = ?", *filter.ActorUserID)
	}
	if value := strings.TrimSpace(filter.ActorRoleKey); value != "" {
		query = query.Where("actor_role_key = ?", value)
	}
	if value := strings.TrimSpace(filter.ResourceType); value != "" {
		query = query.Where("resource_type = ?", value)
	}
	if value := strings.TrimSpace(filter.Action); value != "" {
		query = query.Where("action = ?", value)
	}
	if value := strings.TrimSpace(filter.Result); value != "" {
		query = query.Where("result = ?", value)
	}
	if filter.SourceID != nil {
		query = query.Where("source_id = ?", *filter.SourceID)
	}
	if value := strings.TrimSpace(filter.VirtualPath); value != "" {
		query = query.Where("virtual_path LIKE ?", value+"%")
	}
	if value := strings.TrimSpace(filter.RequestID); value != "" {
		query = query.Where("request_id = ?", value)
	}
	if value := strings.TrimSpace(filter.EntryPoint); value != "" {
		query = query.Where("entry_point = ?", value)
	}
	if filter.StartedAt != nil {
		query = query.Where("occurred_at >= ?", filter.StartedAt.UTC())
	}
	if filter.EndedAt != nil {
		query = query.Where("occurred_at <= ?", filter.EndedAt.UTC())
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := filter.Page
	if page <= 0 {
		page = 1
	}
	pageSize := filter.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}

	var models []AuditLogModel
	if err := query.Order("occurred_at desc, id desc").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&models).Error; err != nil {
		return nil, 0, err
	}

	items := make([]*entity.AuditLog, 0, len(models))
	for index := range models {
		items = append(items, auditLogEntityFromModel(&models[index]))
	}
	return items, int(total), nil
}

func auditLogModelFromEntity(log *entity.AuditLog) *AuditLogModel {
	if log == nil {
		return &AuditLogModel{}
	}
	createdAt := log.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	occurredAt := log.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = createdAt
	}
	return &AuditLogModel{
		ID:               log.ID,
		OccurredAt:       occurredAt,
		RequestID:        log.RequestID,
		EntryPoint:       log.EntryPoint,
		ActorUserID:      log.ActorUserID,
		ActorUsername:    log.ActorUsername,
		ActorRoleKey:     log.ActorRoleKey,
		ClientIP:         log.ClientIP,
		UserAgent:        log.UserAgent,
		Method:           log.Method,
		Path:             log.Path,
		ResourceType:     log.ResourceType,
		Action:           log.Action,
		Result:           log.Result,
		ErrorCode:        log.ErrorCode,
		ResourceID:       log.ResourceID,
		SourceID:         log.SourceID,
		VirtualPath:      log.VirtualPath,
		ResolvedSourceID: log.ResolvedSourceID,
		ResolvedPath:     log.ResolvedPath,
		BeforeJSON:       log.BeforeJSON,
		AfterJSON:        log.AfterJSON,
		DetailJSON:       log.DetailJSON,
		CreatedAt:        createdAt,
	}
}

func auditLogEntityFromModel(model *AuditLogModel) *entity.AuditLog {
	if model == nil {
		return &entity.AuditLog{}
	}
	return &entity.AuditLog{
		ID:               model.ID,
		OccurredAt:       model.OccurredAt,
		RequestID:        model.RequestID,
		EntryPoint:       model.EntryPoint,
		ActorUserID:      model.ActorUserID,
		ActorUsername:    model.ActorUsername,
		ActorRoleKey:     model.ActorRoleKey,
		ClientIP:         model.ClientIP,
		UserAgent:        model.UserAgent,
		Method:           model.Method,
		Path:             model.Path,
		ResourceType:     model.ResourceType,
		Action:           model.Action,
		Result:           model.Result,
		ErrorCode:        model.ErrorCode,
		ResourceID:       model.ResourceID,
		SourceID:         model.SourceID,
		VirtualPath:      model.VirtualPath,
		ResolvedSourceID: model.ResolvedSourceID,
		ResolvedPath:     model.ResolvedPath,
		BeforeJSON:       model.BeforeJSON,
		AfterJSON:        model.AfterJSON,
		DetailJSON:       model.DetailJSON,
		CreatedAt:        model.CreatedAt,
	}
}
