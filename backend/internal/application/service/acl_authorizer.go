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
	metadataReader   metadataVFSReader
}

// ACLAuthorizerOption 定义 ACLAuthorizer 的可选配置。
type ACLAuthorizerOption func(*ACLAuthorizer)

// NewACLAuthorizer 创建 ACL 判定器。
func NewACLAuthorizer(
	systemConfigRepo domainrepo.SystemConfigRepository,
	aclRepo domainrepo.ACLRuleRepository,
	sourceRepo domainrepo.SourceRepository,
	options ...ACLAuthorizerOption,
) *ACLAuthorizer {
	authorizer := &ACLAuthorizer{
		systemConfigRepo: systemConfigRepo,
		aclRepo:          aclRepo,
		sourceRepo:       sourceRepo,
	}
	for _, option := range options {
		option(authorizer)
	}
	return authorizer
}

// WithACLAuthorizerMetadataReader 注入 metadata VFS 读模型用于 node-first 判定。
func WithACLAuthorizerMetadataReader(reader metadataVFSReader) ACLAuthorizerOption {
	return func(a *ACLAuthorizer) {
		a.metadataReader = reader
	}
}

// AuthorizePath 判定当前请求是否允许访问指定路径。
func (a *ACLAuthorizer) AuthorizePath(ctx context.Context, sourceID uint, pathValue string, action ACLAction) error {
	evaluator, err := a.newEvaluator(ctx, sourceID)
	if err != nil {
		return err
	}
	allowed, err := evaluator.allowPath(ctx, pathValue, action)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrACLDenied
	}
	return nil
}

// AuthorizeVirtualPath 按 VFS 虚拟路径/node 判定当前请求是否允许访问。
func (a *ACLAuthorizer) AuthorizeVirtualPath(ctx context.Context, sourceID uint, virtualPath string, nodeID uint, action ACLAction) error {
	evaluator, err := a.newEvaluator(ctx, sourceID)
	if err != nil {
		return err
	}
	allowed, err := evaluator.allowVirtualPathWithNodeID(ctx, virtualPath, nodeID, action)
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
		allowed, allowErr := evaluator.allowPath(ctx, item.Path, ACLActionRead)
		if allowErr != nil {
			return nil, allowErr
		}
		if !allowed {
			continue
		}
		deleteAllowed, allowErr := evaluator.allowPath(ctx, item.Path, ACLActionDelete)
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
		allowed, allowErr := evaluator.allowVirtualPathWithNodeID(ctx, item.Path, item.ID, ACLActionRead)
		if allowErr != nil {
			return nil, allowErr
		}
		if !allowed {
			continue
		}
		deleteAllowed, allowErr := evaluator.allowVirtualPathWithNodeID(ctx, item.Path, item.ID, ACLActionDelete)
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
		if !(rule.Read || rule.Write || rule.Delete || rule.Share) {
			continue
		}
		if evaluator.ruleEffectivelyAllowsSourceExposure(ctx, rule) {
			return true, nil
		}
	}
	return false, nil
}

type aclEvaluator struct {
	bypass         bool
	userID         uint
	sourceID       uint
	rules          []*entity.ACLRule
	mountPath      string
	metadataReader metadataVFSReader
	nodeCache      map[uint]*entity.VFSNode
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
	rules, err := a.aclRepo.List(ctx, domainrepo.ACLRuleFilter{SourceID: sourceID, IncludeGlobal: true})
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
	if a.sourceRepo != nil && sourceID != 0 {
		source, err := a.sourceRepo.FindByID(ctx, sourceID)
		if err != nil {
			return nil, err
		}
		if source.MountPath != "" {
			mountPath = source.MountPath
		}
	}
	return &aclEvaluator{
		userID:         auth.UserID,
		sourceID:       sourceID,
		rules:          rules,
		mountPath:      mountPath,
		metadataReader: a.metadataReader,
		nodeCache:      make(map[uint]*entity.VFSNode),
	}, nil
}

func (e *aclEvaluator) allowPath(ctx context.Context, pathValue string, action ACLAction) (bool, error) {
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
	targetNode := e.resolveTargetNodeBestEffort(ctx, targetVirtualPath, 0)
	return e.allowTarget(ctx, normalizedPath, targetVirtualPath, targetNode, action)
}

func (e *aclEvaluator) allowVirtualPath(ctx context.Context, virtualPath string, action ACLAction) (bool, error) {
	return e.allowVirtualPathWithNodeID(ctx, virtualPath, 0, action)
}

func (e *aclEvaluator) allowVirtualPathWithNodeID(ctx context.Context, virtualPath string, nodeID uint, action ACLAction) (bool, error) {
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

	targetNode := e.resolveTargetNodeBestEffort(ctx, normalizedVirtualPath, nodeID)
	return e.allowTarget(ctx, innerPath, normalizedVirtualPath, targetNode, action)
}

func (e *aclEvaluator) allowTarget(ctx context.Context, targetPath string, targetVirtualPath string, targetNode *entity.VFSNode, action ACLAction) (bool, error) {
	var matchedPriority *int
	allowMatched := false
	for _, rule := range e.rules {
		if rule.SubjectType != "user" || rule.SubjectID != e.userID {
			continue
		}
		if !e.ruleMatchesTarget(ctx, rule, targetPath, targetVirtualPath, targetNode) {
			continue
		}
		if !ruleContainsAction(rule, action) {
			continue
		}
		if matchedPriority != nil && rule.Priority < *matchedPriority {
			break
		}
		if matchedPriority == nil {
			priority := rule.Priority
			matchedPriority = &priority
		}
		if strings.TrimSpace(rule.Effect) == "deny" {
			return false, nil
		}
		if strings.TrimSpace(rule.Effect) == "allow" {
			allowMatched = true
		}
	}
	return allowMatched, nil
}

func (e *aclEvaluator) ruleMatchesTarget(ctx context.Context, rule *entity.ACLRule, targetPath string, targetVirtualPath string, targetNode *entity.VFSNode) bool {
	if rule == nil {
		return false
	}
	if rule.VFSNodeID != nil {
		if e.metadataReader == nil {
			return ruleMatchesPath(rule, targetPath, targetVirtualPath)
		}
		ruleNode := e.resolveRuleNode(ctx, *rule.VFSNodeID)
		if ruleNode == nil {
			return false
		}
		if targetNode != nil {
			return ruleMatchesNode(rule, ruleNode, targetNode)
		}
		return ruleMatchesNodePath(rule, ruleNode, targetVirtualPath)
	}
	return ruleMatchesPath(rule, targetPath, targetVirtualPath)
}

func (e *aclEvaluator) resolveTargetNodeBestEffort(ctx context.Context, virtualPath string, nodeID uint) *entity.VFSNode {
	if e.metadataReader == nil {
		return nil
	}
	if nodeID != 0 {
		return e.resolveRuleNode(ctx, nodeID)
	}
	normalizedPath, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return nil
	}
	node, err := e.metadataReader.ResolveNode(ctx, normalizedPath)
	if err != nil || node == nil || node.IsDeleted {
		return nil
	}
	return node
}

func (e *aclEvaluator) resolveRuleNode(ctx context.Context, nodeID uint) *entity.VFSNode {
	if e.metadataReader == nil || nodeID == 0 {
		return nil
	}
	if cached, ok := e.nodeCache[nodeID]; ok {
		return cached
	}
	node, err := e.metadataReader.ResolveNodeByID(ctx, nodeID)
	if err != nil || node == nil || node.IsDeleted {
		e.nodeCache[nodeID] = nil
		return nil
	}
	e.nodeCache[nodeID] = node
	return node
}

func (e *aclEvaluator) ruleCanExposeSource(ctx context.Context, rule *entity.ACLRule) bool {
	if rule == nil {
		return false
	}
	if rule.SourceID != 0 && rule.SourceID != e.sourceID {
		return false
	}
	return e.sourceExposureVirtualPath(ctx, rule) != ""
}

func (e *aclEvaluator) ruleEffectivelyAllowsSourceExposure(ctx context.Context, rule *entity.ACLRule) bool {
	if !e.ruleCanExposeSource(ctx, rule) {
		return false
	}
	exposurePath := e.sourceExposureVirtualPath(ctx, rule)
	if exposurePath == "" {
		return false
	}
	targetPath := e.innerPathForVirtualPath(exposurePath)
	targetNode := e.resolveTargetNodeBestEffort(ctx, exposurePath, 0)
	for _, action := range aclActionsForRule(rule) {
		allowed, err := e.allowTarget(ctx, targetPath, exposurePath, targetNode, action)
		if err == nil && allowed {
			return true
		}
	}
	return false
}

func (e *aclEvaluator) sourceExposureVirtualPath(ctx context.Context, rule *entity.ACLRule) string {
	rulePath := e.currentRuleVirtualPath(ctx, rule)
	if rulePath == "" {
		return ""
	}
	mountPath := e.mountPath
	if mountPath == "" {
		mountPath = "/"
	}
	if isSubPath(rulePath, mountPath) {
		return mountPath
	}
	if isSubPath(mountPath, rulePath) {
		return rulePath
	}
	return ""
}

func (e *aclEvaluator) currentRuleVirtualPath(ctx context.Context, rule *entity.ACLRule) string {
	if rule == nil {
		return ""
	}
	if rule.VFSNodeID != nil {
		if node := e.resolveRuleNode(ctx, *rule.VFSNodeID); node != nil {
			return node.Path
		}
	}
	if strings.TrimSpace(rule.VirtualPath) != "" {
		return strings.TrimSpace(rule.VirtualPath)
	}
	rulePath := strings.TrimSpace(rule.Path)
	if rule.SourceID != 0 && rule.SourceID == e.sourceID {
		if merged := mergeMountAndInnerPath(e.mountPath, rulePath); merged != "" {
			return merged
		}
	}
	return rulePath
}

func (e *aclEvaluator) innerPathForVirtualPath(virtualPath string) string {
	normalizedPath, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return virtualPath
	}
	if normalizedMountPath, mountErr := normalizeMountPath(e.mountPath); mountErr == nil && normalizedMountPath != "/" && isSubPath(normalizedMountPath, normalizedPath) {
		innerPath := strings.TrimPrefix(normalizedPath, normalizedMountPath)
		if innerPath == "" {
			return "/"
		}
		if !strings.HasPrefix(innerPath, "/") {
			return "/" + innerPath
		}
		return innerPath
	}
	return normalizedPath
}

func ruleMatchesNode(rule *entity.ACLRule, ruleNode *entity.VFSNode, targetNode *entity.VFSNode) bool {
	if rule == nil || ruleNode == nil || targetNode == nil {
		return false
	}
	if ruleNode.ID == targetNode.ID {
		return true
	}
	if !rule.InheritToChildren {
		return false
	}
	return isSubPath(ruleNode.Path, targetNode.Path)
}

func ruleMatchesNodePath(rule *entity.ACLRule, ruleNode *entity.VFSNode, targetVirtualPath string) bool {
	if rule == nil || ruleNode == nil {
		return false
	}
	if ruleNode.Path == targetVirtualPath {
		return true
	}
	if !rule.InheritToChildren {
		return false
	}
	return isSubPath(ruleNode.Path, targetVirtualPath)
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

func aclActionsForRule(rule *entity.ACLRule) []ACLAction {
	if rule == nil {
		return nil
	}
	actions := make([]ACLAction, 0, 4)
	if rule.Read {
		actions = append(actions, ACLActionRead)
	}
	if rule.Write {
		actions = append(actions, ACLActionWrite)
	}
	if rule.Delete {
		actions = append(actions, ACLActionDelete)
	}
	if rule.Share {
		actions = append(actions, ACLActionShare)
	}
	return actions
}
