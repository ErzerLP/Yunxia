package service

import (
	"context"
	"errors"
	"strings"

	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	"yunxia/internal/domain/permission"
	domainrepo "yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/security"
)

// ACLAction 表示 ACL 动作类型。
type ACLAction string

const (
	ACLActionRead   ACLAction = "read"
	ACLActionWrite  ACLAction = "write"
	ACLActionDelete ACLAction = "delete"
	ACLActionShare  ACLAction = "share"
)

// ACLAuthorizer 负责 ACL 运行时判定。
type ACLAuthorizer struct {
	systemConfigRepo domainrepo.SystemConfigRepository
	aclRepo          domainrepo.ACLRuleRepository
	sourceRepo       domainrepo.SourceRepository
}

// NewACLAuthorizer 创建 ACL 判定器。
func NewACLAuthorizer(
	systemConfigRepo domainrepo.SystemConfigRepository,
	aclRepo domainrepo.ACLRuleRepository,
	sourceRepo domainrepo.SourceRepository,
) *ACLAuthorizer {
	return &ACLAuthorizer{
		systemConfigRepo: systemConfigRepo,
		aclRepo:          aclRepo,
		sourceRepo:       sourceRepo,
	}
}

// AuthorizePath 判定当前请求是否允许访问指定路径。
func (a *ACLAuthorizer) AuthorizePath(ctx context.Context, sourceID uint, pathValue string, action ACLAction) error {
	evaluator, err := a.newEvaluator(ctx, sourceID)
	if err != nil {
		return err
	}
	allowed, err := evaluator.allowPath(pathValue, action)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrACLDenied
	}
	return nil
}

// FilterFileItems 按 read 权限过滤文件项。
func (a *ACLAuthorizer) FilterFileItems(ctx context.Context, sourceID uint, items []appdto.FileItem) ([]appdto.FileItem, error) {
	evaluator, err := a.newEvaluator(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	if evaluator.bypass {
		return items, nil
	}

	filtered := make([]appdto.FileItem, 0, len(items))
	for _, item := range items {
		allowed, allowErr := evaluator.allowPath(item.Path, ACLActionRead)
		if allowErr != nil {
			return nil, allowErr
		}
		if !allowed {
			continue
		}
		deleteAllowed, allowErr := evaluator.allowPath(item.Path, ACLActionDelete)
		if allowErr != nil {
			return nil, allowErr
		}
		item.CanDelete = item.CanDelete && deleteAllowed
		item.CanDownload = item.CanDownload && !item.IsDir
		filtered = append(filtered, item)
	}
	return filtered, nil
}

// FilterVFSItems 按 read 权限过滤统一虚拟目录项。
func (a *ACLAuthorizer) FilterVFSItems(ctx context.Context, sourceID uint, items []appdto.VFSItem) ([]appdto.VFSItem, error) {
	evaluator, err := a.newEvaluator(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	if evaluator.bypass {
		return items, nil
	}

	filtered := make([]appdto.VFSItem, 0, len(items))
	for _, item := range items {
		allowed, allowErr := evaluator.allowVirtualPath(item.Path, ACLActionRead)
		if allowErr != nil {
			return nil, allowErr
		}
		if !allowed {
			continue
		}
		deleteAllowed, allowErr := evaluator.allowVirtualPath(item.Path, ACLActionDelete)
		if allowErr != nil {
			return nil, allowErr
		}
		item.CanDelete = item.CanDelete && deleteAllowed
		item.CanDownload = item.CanDownload && item.EntryKind == string(VirtualEntryKindFile)
		filtered = append(filtered, item)
	}
	return filtered, nil
}

// CanSeeSource 判定当前用户是否应在导航中看见该 source。
func (a *ACLAuthorizer) CanSeeSource(ctx context.Context, sourceID uint) (bool, error) {
	evaluator, err := a.newEvaluator(ctx, sourceID)
	if err != nil {
		return false, err
	}
	if evaluator.bypass {
		return true, nil
	}
	for _, rule := range evaluator.rules {
		if rule.SubjectType != "user" || rule.SubjectID != evaluator.userID {
			continue
		}
		if strings.TrimSpace(rule.Effect) != "allow" {
			continue
		}
		if rule.Read || rule.Write || rule.Delete || rule.Share {
			return true, nil
		}
	}
	return false, nil
}

type aclEvaluator struct {
	bypass    bool
	userID    uint
	rules     []*entity.ACLRule
	mountPath string
}

func (a *ACLAuthorizer) newEvaluator(ctx context.Context, sourceID uint) (*aclEvaluator, error) {
	if a == nil || a.systemConfigRepo == nil || a.aclRepo == nil {
		return &aclEvaluator{bypass: true}, nil
	}
	auth, ok := security.RequestAuthFromContext(ctx)
	if !ok {
		return &aclEvaluator{bypass: true}, nil
	}
	if auth.RoleKey == permission.RoleSuperAdmin {
		return &aclEvaluator{bypass: true}, nil
	}
	rules, err := a.aclRepo.List(ctx, domainrepo.ACLRuleFilter{SourceID: sourceID})
	if err != nil {
		return nil, err
	}
	cfg, err := a.systemConfigRepo.Get(ctx)
	if err != nil {
		if errors.Is(err, domainrepo.ErrNotFound) {
			return &aclEvaluator{bypass: len(rules) == 0}, nil
		}
		return nil, err
	}
	if !cfg.MultiUserEnabled && len(rules) == 0 {
		return &aclEvaluator{bypass: true}, nil
	}
	mountPath := "/"
	if a.sourceRepo != nil {
		source, err := a.sourceRepo.FindByID(ctx, sourceID)
		if err != nil {
			return nil, err
		}
		if source.MountPath != "" {
			mountPath = source.MountPath
		}
	}
	return &aclEvaluator{
		userID:    auth.UserID,
		rules:     rules,
		mountPath: mountPath,
	}, nil
}

func (e *aclEvaluator) allowPath(pathValue string, action ACLAction) (bool, error) {
	if e.bypass {
		return true, nil
	}
	normalizedPath, err := normalizeVirtualPath(pathValue)
	if err != nil {
		return false, ErrPathInvalid
	}
	targetVirtualPath := mergeMountAndInnerPath(e.mountPath, normalizedPath)
	if targetVirtualPath == "" {
		targetVirtualPath = normalizedPath
	}
	for _, rule := range e.rules {
		if rule.SubjectType != "user" || rule.SubjectID != e.userID {
			continue
		}
		if !ruleMatchesPath(rule, normalizedPath, targetVirtualPath) {
			continue
		}
		if !ruleContainsAction(rule, action) {
			continue
		}
		return strings.TrimSpace(rule.Effect) == "allow", nil
	}
	return false, nil
}

func (e *aclEvaluator) allowVirtualPath(virtualPath string, action ACLAction) (bool, error) {
	if e.bypass {
		return true, nil
	}
	normalizedVirtualPath, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return false, ErrPathInvalid
	}

	innerPath := normalizedVirtualPath
	if normalizedMountPath, mountErr := normalizeMountPath(e.mountPath); mountErr == nil && normalizedMountPath != "/" {
		if !isSubPath(normalizedMountPath, normalizedVirtualPath) {
			return false, nil
		}
		innerPath = strings.TrimPrefix(normalizedVirtualPath, normalizedMountPath)
		if innerPath == "" {
			innerPath = "/"
		}
	}

	for _, rule := range e.rules {
		if rule.SubjectType != "user" || rule.SubjectID != e.userID {
			continue
		}
		if !ruleMatchesPath(rule, innerPath, normalizedVirtualPath) {
			continue
		}
		if !ruleContainsAction(rule, action) {
			continue
		}
		return strings.TrimSpace(rule.Effect) == "allow", nil
	}
	return false, nil
}

func ruleMatchesPath(rule *entity.ACLRule, targetPath string, targetVirtualPath string) bool {
	if rule == nil {
		return false
	}
	rulePath := strings.TrimSpace(rule.VirtualPath)
	if rulePath == "" {
		rulePath = strings.TrimSpace(rule.Path)
		targetVirtualPath = targetPath
	}
	if rulePath == targetVirtualPath {
		return true
	}
	if !rule.InheritToChildren {
		return false
	}
	if rulePath == "/" {
		return strings.HasPrefix(targetVirtualPath, "/")
	}
	return strings.HasPrefix(targetVirtualPath, strings.TrimSuffix(rulePath, "/")+"/")
}

func ruleContainsAction(rule *entity.ACLRule, action ACLAction) bool {
	switch action {
	case ACLActionRead:
		return rule.Read
	case ACLActionWrite:
		return rule.Write
	case ACLActionDelete:
		return rule.Delete
	case ACLActionShare:
		return rule.Share
	default:
		return false
	}
}
