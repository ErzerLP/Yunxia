package service

import (
	"context"
	"log/slog"
	"strings"

	appaudit "yunxia/internal/application/audit"
	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	domainrepo "yunxia/internal/domain/repository"
)

// ACLService 负责 ACL 规则管理。
type ACLService struct {
	sourceRepo    domainrepo.SourceRepository
	userRepo      domainrepo.UserRepository
	aclRepo       domainrepo.ACLRuleRepository
	logger        *slog.Logger
	auditRecorder *appaudit.Recorder
}

// NewACLService 创建 ACL 服务。
func NewACLService(
	sourceRepo domainrepo.SourceRepository,
	userRepo domainrepo.UserRepository,
	aclRepo domainrepo.ACLRuleRepository,
	options ...ACLServiceOption,
) *ACLService {
	service := &ACLService{
		sourceRepo: sourceRepo,
		userRepo:   userRepo,
		aclRepo:    aclRepo,
		logger:     newServiceLogger("service.acl"),
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// List 返回 ACL 规则列表。
func (s *ACLService) List(ctx context.Context, query appdto.ACLRuleListQuery) (*appdto.ACLRuleListResponse, error) {
	if _, err := s.sourceRepo.FindByID(ctx, query.SourceID); err != nil {
		return nil, err
	}
	filterPath := ""
	if strings.TrimSpace(query.Path) != "" {
		pathValue, err := normalizeVirtualPath(query.Path)
		if err != nil {
			return nil, ErrPathInvalid
		}
		filterPath = pathValue
	}

	items, err := s.aclRepo.List(ctx, domainrepo.ACLRuleFilter{
		SourceID: query.SourceID,
		Path:     filterPath,
	})
	if err != nil {
		return nil, err
	}
	views := make([]appdto.ACLRuleView, 0, len(items))
	for _, item := range items {
		views = append(views, toACLRuleView(item))
	}
	return &appdto.ACLRuleListResponse{Items: views}, nil
}

// Create 创建 ACL 规则。
func (s *ACLService) Create(ctx context.Context, req appdto.CreateACLRuleRequest) (*appdto.ACLRuleView, error) {
	rule, err := s.buildRuleEntity(ctx, req.SourceID, req.Path, req.SubjectType, req.SubjectID, req.Effect, req.Priority, req.Permissions, req.InheritToChildren)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "acl_rule",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    aclErrorCode(err),
		})
		return nil, err
	}
	if err := s.aclRepo.Create(ctx, rule); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "acl_rule",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
		})
		return nil, err
	}
	view := toACLRuleView(rule)
	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "acl_rule",
		Action:       "create",
		Result:       appaudit.ResultSuccess,
		ResourceID:   encodeUintID(rule.ID),
		SourceID:     &rule.SourceID,
		VirtualPath:  rule.VirtualPath,
		After:        aclRuleAuditView(rule),
	})
	return &view, nil
}

// Update 更新 ACL 规则。
func (s *ACLService) Update(ctx context.Context, id uint, req appdto.UpdateACLRuleRequest) (*appdto.ACLRuleView, error) {
	current, err := s.aclRepo.FindByID(ctx, id)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "acl_rule",
			Action:       "update",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "ACL_RULE_NOT_FOUND",
			ResourceID:   encodeUintID(id),
		})
		return nil, err
	}
	before := aclRuleAuditView(current)
	rule, err := s.buildRuleEntity(ctx, current.SourceID, req.Path, req.SubjectType, req.SubjectID, req.Effect, req.Priority, req.Permissions, req.InheritToChildren)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "acl_rule",
			Action:       "update",
			Result:       appaudit.ResultFailed,
			ErrorCode:    aclErrorCode(err),
			ResourceID:   encodeUintID(id),
			Before:       before,
		})
		return nil, err
	}
	rule.ID = current.ID
	rule.CreatedAt = current.CreatedAt
	if err := s.aclRepo.Update(ctx, rule); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "acl_rule",
			Action:       "update",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
			ResourceID:   encodeUintID(id),
			Before:       before,
		})
		return nil, err
	}
	view := toACLRuleView(rule)
	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "acl_rule",
		Action:       "update",
		Result:       appaudit.ResultSuccess,
		ResourceID:   encodeUintID(id),
		SourceID:     &rule.SourceID,
		VirtualPath:  rule.VirtualPath,
		Before:       before,
		After:        aclRuleAuditView(rule),
	})
	return &view, nil
}

// Delete 删除 ACL 规则。
func (s *ACLService) Delete(ctx context.Context, id uint) error {
	current, err := s.aclRepo.FindByID(ctx, id)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "acl_rule",
			Action:       "delete",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "ACL_RULE_NOT_FOUND",
			ResourceID:   encodeUintID(id),
		})
		return err
	}
	before := aclRuleAuditView(current)
	if err := s.aclRepo.Delete(ctx, id); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "acl_rule",
			Action:       "delete",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
			ResourceID:   encodeUintID(id),
			Before:       before,
		})
		return err
	}
	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "acl_rule",
		Action:       "delete",
		Result:       appaudit.ResultSuccess,
		ResourceID:   encodeUintID(id),
		SourceID:     &current.SourceID,
		VirtualPath:  current.VirtualPath,
		Before:       before,
	})
	return nil
}

func (s *ACLService) buildRuleEntity(
	ctx context.Context,
	sourceID uint,
	pathValue string,
	subjectType string,
	subjectID uint,
	effect string,
	priority int,
	permissions appdto.ACLPermissions,
	inheritToChildren bool,
) (*entity.ACLRule, error) {
	source, err := s.sourceRepo.FindByID(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	normalizedPath, err := normalizeVirtualPath(pathValue)
	if err != nil {
		return nil, ErrPathInvalid
	}
	if strings.TrimSpace(subjectType) != "user" {
		return nil, ErrACLSubjectTypeInvalid
	}
	if _, err := s.userRepo.FindByID(ctx, subjectID); err != nil {
		return nil, err
	}
	switch strings.TrimSpace(effect) {
	case "allow", "deny":
	default:
		return nil, ErrACLEffectInvalid
	}
	if !permissions.Read && !permissions.Write && !permissions.Delete && !permissions.Share {
		return nil, ErrACLPermissionsInvalid
	}
	virtualPath := mergeMountAndInnerPath(source.MountPath, normalizedPath)
	if virtualPath == "" {
		virtualPath = normalizedPath
	}

	return &entity.ACLRule{
		SourceID:          sourceID,
		Path:              normalizedPath,
		VirtualPath:       virtualPath,
		SubjectType:       "user",
		SubjectID:         subjectID,
		Effect:            strings.TrimSpace(effect),
		Priority:          priority,
		Read:              permissions.Read,
		Write:             permissions.Write,
		Delete:            permissions.Delete,
		Share:             permissions.Share,
		InheritToChildren: inheritToChildren,
	}, nil
}

func toACLRuleView(rule *entity.ACLRule) appdto.ACLRuleView {
	return appdto.ACLRuleView{
		ID:          rule.ID,
		SourceID:    rule.SourceID,
		Path:        rule.Path,
		VirtualPath: rule.VirtualPath,
		SubjectType: rule.SubjectType,
		SubjectID:   rule.SubjectID,
		Effect:      rule.Effect,
		Priority:    rule.Priority,
		Permissions: appdto.ACLPermissions{
			Read:   rule.Read,
			Write:  rule.Write,
			Delete: rule.Delete,
			Share:  rule.Share,
		},
		InheritToChildren: rule.InheritToChildren,
	}
}
