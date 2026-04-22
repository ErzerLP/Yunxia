package repository

import (
	"context"

	"yunxia/internal/domain/entity"
)

// ACLRuleFilter 定义 ACL 规则查询条件。
type ACLRuleFilter struct {
	SourceID uint
	Path     string
}

// ACLRuleRepository 定义 ACL 规则持久化能力。
type ACLRuleRepository interface {
	Create(ctx context.Context, rule *entity.ACLRule) error
	FindByID(ctx context.Context, id uint) (*entity.ACLRule, error)
	List(ctx context.Context, filter ACLRuleFilter) ([]*entity.ACLRule, error)
	Update(ctx context.Context, rule *entity.ACLRule) error
	Delete(ctx context.Context, id uint) error
}
