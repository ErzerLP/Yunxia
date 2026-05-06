package gorm

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

// ACLRuleRepository 提供 ACL 规则仓储实现。
type ACLRuleRepository struct {
	db *gorm.DB
}

// NewACLRuleRepository 创建 ACL 规则仓储。
func NewACLRuleRepository(db *gorm.DB) *ACLRuleRepository {
	return &ACLRuleRepository{db: db}
}

// Create 创建规则。
func (r *ACLRuleRepository) Create(ctx context.Context, rule *entity.ACLRule) error {
	model := aclRuleModelFromEntity(rule)
	if err := dbFor(ctx, r.db).Create(model).Error; err != nil {
		return normalizeGormError(err)
	}
	*rule = *aclRuleEntityFromModel(model)
	return nil
}

// FindByID 按 ID 查询规则。
func (r *ACLRuleRepository) FindByID(ctx context.Context, id uint) (*entity.ACLRule, error) {
	var model ACLRuleModel
	if err := dbFor(ctx, r.db).First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainrepo.ErrNotFound
		}
		return nil, normalizeGormError(err)
	}
	return aclRuleEntityFromModel(&model), nil
}

// List 返回筛选后的规则列表。
func (r *ACLRuleRepository) List(ctx context.Context, filter domainrepo.ACLRuleFilter) ([]*entity.ACLRule, error) {
	query := dbFor(ctx, r.db).Model(&ACLRuleModel{}).Where("source_id = ?", filter.SourceID)
	if path := strings.TrimSpace(filter.Path); path != "" {
		query = query.Where("path = ?", path)
	}

	var models []ACLRuleModel
	if err := query.Order("priority desc, id asc").Find(&models).Error; err != nil {
		return nil, normalizeGormError(err)
	}
	items := make([]*entity.ACLRule, 0, len(models))
	for index := range models {
		items = append(items, aclRuleEntityFromModel(&models[index]))
	}
	return items, nil
}

// Update 更新规则。
func (r *ACLRuleRepository) Update(ctx context.Context, rule *entity.ACLRule) error {
	model := aclRuleModelFromEntity(rule)
	result := dbFor(ctx, r.db).
		Model(&ACLRuleModel{}).
		Where("id = ?", rule.ID).
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

// Delete 删除规则。
func (r *ACLRuleRepository) Delete(ctx context.Context, id uint) error {
	result := dbFor(ctx, r.db).Delete(&ACLRuleModel{}, id)
	if result.Error != nil {
		return normalizeGormError(result.Error)
	}
	if result.RowsAffected == 0 {
		return domainrepo.ErrNotFound
	}
	return nil
}

func aclRuleModelFromEntity(rule *entity.ACLRule) *ACLRuleModel {
	return &ACLRuleModel{
		ID:                rule.ID,
		SourceID:          rule.SourceID,
		Path:              rule.Path,
		VirtualPath:       rule.VirtualPath,
		SubjectType:       rule.SubjectType,
		SubjectID:         rule.SubjectID,
		Effect:            rule.Effect,
		Priority:          rule.Priority,
		Read:              rule.Read,
		Write:             rule.Write,
		Delete:            rule.Delete,
		Share:             rule.Share,
		InheritToChildren: rule.InheritToChildren,
		CreatedAt:         rule.CreatedAt,
		UpdatedAt:         rule.UpdatedAt,
	}
}

func aclRuleEntityFromModel(model *ACLRuleModel) *entity.ACLRule {
	return &entity.ACLRule{
		ID:                model.ID,
		SourceID:          model.SourceID,
		Path:              model.Path,
		VirtualPath:       model.VirtualPath,
		SubjectType:       model.SubjectType,
		SubjectID:         model.SubjectID,
		Effect:            model.Effect,
		Priority:          model.Priority,
		Read:              model.Read,
		Write:             model.Write,
		Delete:            model.Delete,
		Share:             model.Share,
		InheritToChildren: model.InheritToChildren,
		CreatedAt:         model.CreatedAt,
		UpdatedAt:         model.UpdatedAt,
	}
}
