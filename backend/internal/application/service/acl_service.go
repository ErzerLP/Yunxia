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
	sourceRepo     domainrepo.SourceRepository
	userRepo       domainrepo.UserRepository
	aclRepo        domainrepo.ACLRuleRepository
	metadataReader metadataVFSReader
	logger         *slog.Logger
	auditRecorder  *appaudit.Recorder
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
	anySource := false
	if query.SourceID > 0 {
		if _, err := s.sourceRepo.FindByID(ctx, query.SourceID); err != nil {
			return nil, err
		}
	} else if query.VFSNodeID != nil {
		anySource = true
	} else {
		return nil, ErrPathInvalid
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
		SourceID:  query.SourceID,
		Path:      filterPath,
		VFSNodeID: query.VFSNodeID,
		AnySource: anySource,
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
	rule, err := s.buildRuleEntity(ctx, aclRuleBuildInput{
		SourceID:          req.SourceID,
		VFSNodeID:         req.VFSNodeID,
		Path:              req.Path,
		SubjectType:       req.SubjectType,
		SubjectID:         req.SubjectID,
		Effect:            req.Effect,
		Priority:          req.Priority,
		Permissions:       req.Permissions,
		InheritToChildren: req.InheritToChildren,
	})
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
	sourceID := current.SourceID
	if req.SourceID > 0 {
		sourceID = req.SourceID
	}
	rule, err := s.buildRuleEntity(ctx, aclRuleBuildInput{
		SourceID:          sourceID,
		VFSNodeID:         req.VFSNodeID,
		Path:              req.Path,
		SubjectType:       req.SubjectType,
		SubjectID:         req.SubjectID,
		Effect:            req.Effect,
		Priority:          req.Priority,
		Permissions:       req.Permissions,
		InheritToChildren: req.InheritToChildren,
	})
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

type aclRuleBuildInput struct {
	SourceID          uint
	VFSNodeID         *uint
	Path              string
	SubjectType       string
	SubjectID         uint
	Effect            string
	Priority          int
	Permissions       appdto.ACLPermissions
	InheritToChildren bool
}

func (s *ACLService) buildRuleEntity(ctx context.Context, input aclRuleBuildInput) (*entity.ACLRule, error) {
	if strings.TrimSpace(input.SubjectType) != "user" {
		return nil, ErrACLSubjectTypeInvalid
	}
	if _, err := s.userRepo.FindByID(ctx, input.SubjectID); err != nil {
		return nil, err
	}
	effect := strings.TrimSpace(input.Effect)
	switch effect {
	case "allow", "deny":
	default:
		return nil, ErrACLEffectInvalid
	}
	if !input.Permissions.Read && !input.Permissions.Write && !input.Permissions.Delete && !input.Permissions.Share {
		return nil, ErrACLPermissionsInvalid
	}

	sourceID := input.SourceID
	var source *entity.StorageSource
	var node *entity.VFSNode
	if input.VFSNodeID != nil {
		resolved, err := s.resolveACLRuleNode(ctx, *input.VFSNodeID)
		if err != nil {
			return nil, err
		}
		node = resolved
		if node.SourceID != nil {
			if sourceID != 0 && sourceID != *node.SourceID {
				return nil, ErrPathInvalid
			}
			sourceID = *node.SourceID
		}
	}
	if sourceID > 0 {
		resolvedSource, err := s.sourceRepo.FindByID(ctx, sourceID)
		if err != nil {
			return nil, err
		}
		source = resolvedSource
	} else if node == nil {
		return nil, ErrPathInvalid
	}

	normalizedPath := ""
	virtualPath := ""
	vfsNodeID := cloneUintPtr(input.VFSNodeID)
	if node != nil {
		virtualPath = node.Path
		normalizedPath = aclInnerPathSnapshotForNode(node, source)
	} else {
		if strings.TrimSpace(input.Path) == "" {
			return nil, ErrPathInvalid
		}
		pathValue, err := normalizeVirtualPath(input.Path)
		if err != nil {
			return nil, ErrPathInvalid
		}
		normalizedPath = pathValue
		virtualPath = normalizedPath
		if source != nil {
			virtualPath = mergeMountAndInnerPath(source.MountPath, normalizedPath)
			if virtualPath == "" {
				virtualPath = normalizedPath
			}
		}
		vfsNodeID = s.resolveACLRuleNodeIDBestEffort(ctx, virtualPath)
	}

	return &entity.ACLRule{
		SourceID:          sourceID,
		VFSNodeID:         vfsNodeID,
		Path:              normalizedPath,
		VirtualPath:       virtualPath,
		SubjectType:       "user",
		SubjectID:         input.SubjectID,
		Effect:            effect,
		Priority:          input.Priority,
		Read:              input.Permissions.Read,
		Write:             input.Permissions.Write,
		Delete:            input.Permissions.Delete,
		Share:             input.Permissions.Share,
		InheritToChildren: input.InheritToChildren,
	}, nil
}

func (s *ACLService) resolveACLRuleNode(ctx context.Context, nodeID uint) (*entity.VFSNode, error) {
	if nodeID == 0 || s.metadataReader == nil {
		return nil, ErrFileNotFound
	}
	node, err := s.metadataReader.ResolveNodeByID(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	if node == nil || node.IsDeleted {
		return nil, ErrFileNotFound
	}
	return node, nil
}

func (s *ACLService) resolveACLRuleNodeIDBestEffort(ctx context.Context, virtualPath string) *uint {
	if s.metadataReader == nil || strings.TrimSpace(virtualPath) == "" {
		return nil
	}
	nodeID := resolveMetadataVFSNodeID(ctx, s.metadataReader, virtualPath)
	if nodeID == 0 {
		return nil
	}
	return &nodeID
}

func aclInnerPathSnapshotForNode(node *entity.VFSNode, source *entity.StorageSource) string {
	if node == nil {
		return "/"
	}
	normalizedNodePath, err := normalizeVirtualPath(node.Path)
	if err != nil {
		return node.Path
	}
	if source == nil {
		return normalizedNodePath
	}
	normalizedMountPath, err := normalizeMountPath(source.MountPath)
	if err != nil || !isSubPath(normalizedMountPath, normalizedNodePath) {
		return normalizedNodePath
	}
	innerPath := strings.TrimPrefix(normalizedNodePath, normalizedMountPath)
	if innerPath == "" {
		return "/"
	}
	if !strings.HasPrefix(innerPath, "/") {
		innerPath = "/" + innerPath
	}
	return innerPath
}

func toACLRuleView(rule *entity.ACLRule) appdto.ACLRuleView {
	return appdto.ACLRuleView{
		ID:          rule.ID,
		SourceID:    rule.SourceID,
		VFSNodeID:   cloneUintPtr(rule.VFSNodeID),
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
