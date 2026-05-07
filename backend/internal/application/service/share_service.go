package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	appaudit "yunxia/internal/application/audit"
	appdto "yunxia/internal/application/dto"
	"yunxia/internal/domain/entity"
	"yunxia/internal/domain/permission"
	domainrepo "yunxia/internal/domain/repository"
	"yunxia/internal/infrastructure/security"
)

// ShareService 负责分享链接管理与公开访问。
type ShareService struct {
	shareRepo        domainrepo.ShareRepository
	sourceRepo       domainrepo.SourceRepository
	aclAuthorizer    *ACLAuthorizer
	hasher           passwordHasher
	fileAccessTokens interface {
		Issue(sourceID uint, path, purpose, disposition string, ttl time.Duration) (string, time.Time, error)
	}
	fileDrivers     map[string]FileDriver
	metadataReader  metadataVFSReader
	metadataRefresh metadataVFSRefreshService
	now             func() time.Time
	logger          *slog.Logger
	auditRecorder   *appaudit.Recorder
}

// ShareOpenResult 表示公开分享访问结果。
type ShareOpenResult struct {
	RedirectURL string
	Data        *appdto.PublicShareOpenResponse
}

type shareTargetResolution struct {
	Node        *entity.VFSNode
	Source      *entity.StorageSource
	VirtualPath string
	InnerPath   string
	Name        string
	IsDir       bool
	NodeFirst   bool
}

// NewShareService 创建分享服务。
func NewShareService(
	shareRepo domainrepo.ShareRepository,
	sourceRepo domainrepo.SourceRepository,
	hasher passwordHasher,
	fileAccessTokens interface {
		Issue(sourceID uint, path, purpose, disposition string, ttl time.Duration) (string, time.Time, error)
	},
	options ...ShareServiceOption,
) *ShareService {
	service := &ShareService{
		shareRepo:        shareRepo,
		sourceRepo:       sourceRepo,
		hasher:           hasher,
		fileAccessTokens: fileAccessTokens,
		fileDrivers:      make(map[string]FileDriver),
		now:              time.Now,
		logger:           newServiceLogger("service.share"),
	}
	for _, option := range options {
		option(service)
	}
	return service
}

// List 返回当前用户创建的分享链接。
func (s *ShareService) List(ctx context.Context) (*appdto.ShareListResponse, error) {
	auth, ok := security.RequestAuthFromContext(ctx)
	if !ok {
		return nil, ErrPermissionDenied
	}
	var items []*entity.ShareLink
	var err error
	if permission.HasCapability(auth.Capabilities, permission.CapabilityShareReadAll) {
		items, err = s.shareRepo.ListAll(ctx)
	} else {
		items, err = s.shareRepo.ListByUser(ctx, auth.UserID)
	}
	if err != nil {
		return nil, err
	}

	views := make([]appdto.ShareView, 0, len(items))
	for _, item := range items {
		views = append(views, toShareView(item))
	}
	return &appdto.ShareListResponse{Items: views}, nil
}

// Get 返回单个分享详情。
func (s *ShareService) Get(ctx context.Context, id uint) (*appdto.ShareView, error) {
	share, err := s.shareRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeShareOwnership(ctx, share, false); err != nil {
		return nil, err
	}
	view := toShareView(share)
	return &view, nil
}

// Create 创建新的分享链接。
func (s *ShareService) Create(ctx context.Context, req appdto.CreateShareRequest) (*appdto.ShareView, error) {
	source, err := s.sourceRepo.FindByID(ctx, req.SourceID)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "share",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "SOURCE_NOT_FOUND",
			SourceID:     &req.SourceID,
		})
		return nil, err
	}
	if err := s.authorizeSharePath(ctx, source.ID, req.Path); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "share",
			Action:       "create",
			Result:       appaudit.ResultDenied,
			ErrorCode:    "PERMISSION_DENIED",
			SourceID:     &source.ID,
			VirtualPath:  mergeMountAndInnerPath(source.MountPath, req.Path),
		})
		return nil, err
	}

	virtualPath, name, isDir, err := s.inspectTarget(ctx, source, req.Path)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "share",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    shareErrorCode(err),
			SourceID:     &source.ID,
			VirtualPath:  mergeMountAndInnerPath(source.MountPath, req.Path),
		})
		return nil, err
	}

	userID, err := s.currentUserID(ctx)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "share",
			Action:       "create",
			Result:       appaudit.ResultDenied,
			ErrorCode:    "PERMISSION_DENIED",
			SourceID:     &source.ID,
			VirtualPath:  mergeMountAndInnerPath(source.MountPath, virtualPath),
		})
		return nil, err
	}

	var passwordHash *string
	if req.Password != "" {
		hashed, hashErr := s.hasher.Hash(req.Password)
		if hashErr != nil {
			recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
				ResourceType: "share",
				Action:       "create",
				Result:       appaudit.ResultFailed,
				ErrorCode:    "INTERNAL_ERROR",
				SourceID:     &source.ID,
				VirtualPath:  mergeMountAndInnerPath(source.MountPath, virtualPath),
			})
			return nil, hashErr
		}
		passwordHash = &hashed
	}

	now := s.now()
	var expiresAt *time.Time
	if req.ExpiresIn > 0 {
		expireValue := now.Add(time.Duration(req.ExpiresIn) * time.Second)
		expiresAt = &expireValue
	}
	targetVirtualPath := mergeMountAndInnerPath(source.MountPath, virtualPath)
	if targetVirtualPath == "" {
		targetVirtualPath = virtualPath
	}
	targetVFSNodeID, err := s.resolveShareTargetVFSNodeID(ctx, targetVirtualPath)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "share",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    shareErrorCode(err),
			SourceID:     &source.ID,
			VirtualPath:  targetVirtualPath,
		})
		return nil, err
	}

	share := &entity.ShareLink{
		UserID:            userID,
		SourceID:          source.ID,
		Path:              virtualPath,
		TargetVFSNodeID:   targetVFSNodeID,
		TargetVirtualPath: targetVirtualPath,
		ResolvedSourceID:  source.ID,
		ResolvedInnerPath: virtualPath,
		Name:              name,
		IsDir:             isDir,
		Token:             uuid.NewString(),
		PasswordHash:      passwordHash,
		ExpiresAt:         expiresAt,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.shareRepo.Create(ctx, share); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "share",
			Action:       "create",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
			ResourceID:   encodeUintID(share.ID),
			SourceID:     &source.ID,
			VirtualPath:  targetVirtualPath,
		})
		return nil, err
	}

	view := toShareView(share)
	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "share",
		Action:       "create",
		Result:       appaudit.ResultSuccess,
		ResourceID:   encodeUintID(share.ID),
		SourceID:     &source.ID,
		VirtualPath:  targetVirtualPath,
		After:        shareAuditView(share),
	})
	return &view, nil
}

// Update 更新当前用户拥有的分享链接。
func (s *ShareService) Update(ctx context.Context, id uint, req appdto.UpdateShareRequest) (*appdto.ShareView, error) {
	share, err := s.shareRepo.FindByID(ctx, id)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "share",
			Action:       "update",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "SHARE_NOT_FOUND",
			ResourceID:   encodeUintID(id),
		})
		return nil, err
	}
	if err := s.authorizeShareOwnership(ctx, share, true); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "share",
			Action:       "update",
			Result:       appaudit.ResultDenied,
			ErrorCode:    "PERMISSION_DENIED",
			ResourceID:   encodeUintID(id),
			SourceID:     &share.SourceID,
			VirtualPath:  share.TargetVirtualPath,
			Before:       shareAuditView(share),
		})
		return nil, err
	}

	before := shareAuditView(share)
	changed := false
	if req.Password != nil {
		if *req.Password == "" {
			share.PasswordHash = nil
		} else {
			hashed, hashErr := s.hasher.Hash(*req.Password)
			if hashErr != nil {
				recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
					ResourceType: "share",
					Action:       "update",
					Result:       appaudit.ResultFailed,
					ErrorCode:    "INTERNAL_ERROR",
					ResourceID:   encodeUintID(id),
					SourceID:     &share.SourceID,
					VirtualPath:  share.TargetVirtualPath,
					Before:       before,
				})
				return nil, hashErr
			}
			share.PasswordHash = &hashed
		}
		changed = true
	}
	if req.ExpiresIn != nil {
		if *req.ExpiresIn > 0 {
			expireValue := s.now().Add(time.Duration(*req.ExpiresIn) * time.Second)
			share.ExpiresAt = &expireValue
		} else {
			share.ExpiresAt = nil
		}
		changed = true
	}
	if changed {
		share.UpdatedAt = s.now()
		if err := s.shareRepo.Update(ctx, share); err != nil {
			recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
				ResourceType: "share",
				Action:       "update",
				Result:       appaudit.ResultFailed,
				ErrorCode:    "INTERNAL_ERROR",
				ResourceID:   encodeUintID(id),
				SourceID:     &share.SourceID,
				VirtualPath:  share.TargetVirtualPath,
				Before:       before,
			})
			return nil, err
		}
	}

	view := toShareView(share)
	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "share",
		Action:       "update",
		Result:       appaudit.ResultSuccess,
		ResourceID:   encodeUintID(id),
		SourceID:     &share.SourceID,
		VirtualPath:  share.TargetVirtualPath,
		Before:       before,
		After:        shareAuditView(share),
	})
	return &view, nil
}

// Delete 删除当前用户拥有的分享链接。
func (s *ShareService) Delete(ctx context.Context, id uint) (*appdto.DeleteShareResponse, error) {
	share, err := s.shareRepo.FindByID(ctx, id)
	if err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "share",
			Action:       "delete",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "SHARE_NOT_FOUND",
			ResourceID:   encodeUintID(id),
		})
		return nil, err
	}
	if err := s.authorizeShareOwnership(ctx, share, true); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "share",
			Action:       "delete",
			Result:       appaudit.ResultDenied,
			ErrorCode:    "PERMISSION_DENIED",
			ResourceID:   encodeUintID(id),
			SourceID:     &share.SourceID,
			VirtualPath:  share.TargetVirtualPath,
			Before:       shareAuditView(share),
		})
		return nil, err
	}
	if err := s.shareRepo.Delete(ctx, id); err != nil {
		recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
			ResourceType: "share",
			Action:       "delete",
			Result:       appaudit.ResultFailed,
			ErrorCode:    "INTERNAL_ERROR",
			ResourceID:   encodeUintID(id),
			SourceID:     &share.SourceID,
			VirtualPath:  share.TargetVirtualPath,
			Before:       shareAuditView(share),
		})
		return nil, err
	}
	recordServiceAudit(ctx, s.logger, s.auditRecorder, appaudit.Event{
		ResourceType: "share",
		Action:       "delete",
		Result:       appaudit.ResultSuccess,
		ResourceID:   encodeUintID(id),
		SourceID:     &share.SourceID,
		VirtualPath:  share.TargetVirtualPath,
		Before:       shareAuditView(share),
	})
	return &appdto.DeleteShareResponse{
		ID:      id,
		Deleted: true,
	}, nil
}

// Open 解析公开分享链接并返回公开目录数据或下载地址。
func (s *ShareService) Open(ctx context.Context, token, password, relativePath, disposition, sortBy, sortOrder string, page, pageSize int) (*ShareOpenResult, error) {
	share, err := s.shareRepo.FindByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if err := s.ensureShareAccessible(share, password); err != nil {
		return nil, err
	}

	rootTarget, err := s.resolveShareTarget(ctx, share)
	if err != nil {
		return nil, err
	}

	if !rootTarget.IsDir {
		redirectURL, redirectErr := s.buildShareDownloadURL(rootTarget.Source.ID, rootTarget.InnerPath, rootTarget.VirtualPath, disposition, share.ExpiresAt)
		if redirectErr != nil {
			return nil, redirectErr
		}
		return &ShareOpenResult{RedirectURL: redirectURL}, nil
	}

	actualPath, currentPath, err := resolveShareTargetPath(rootTarget.VirtualPath, relativePath)
	if err != nil {
		return nil, err
	}

	currentTarget, err := s.resolveSharePathTarget(ctx, rootTarget, actualPath)
	if err != nil {
		return nil, err
	}
	if !currentTarget.IsDir {
		redirectURL, redirectErr := s.buildShareDownloadURL(currentTarget.Source.ID, currentTarget.InnerPath, currentTarget.VirtualPath, disposition, share.ExpiresAt)
		if redirectErr != nil {
			return nil, redirectErr
		}
		return &ShareOpenResult{RedirectURL: redirectURL}, nil
	}

	items, total, totalPages, err := s.listPublicDirectory(ctx, rootTarget, currentTarget, sortBy, sortOrder, page, pageSize)
	if err != nil {
		return nil, err
	}

	return &ShareOpenResult{
		Data: &appdto.PublicShareOpenResponse{
			Share:       toShareViewWithTarget(share, rootTarget),
			CurrentPath: currentPath,
			CurrentDir:  buildPublicShareCurrentDir(rootTarget.Name, currentPath),
			Breadcrumbs: buildPublicShareBreadcrumbs(rootTarget.Name, currentPath),
			Pagination: appdto.PublicSharePagination{
				Page:       pageValue(page),
				PageSize:   pageSizeValue(pageSize),
				Total:      total,
				TotalPages: totalPages,
			},
			Items: items,
		},
	}, nil
}

func (s *ShareService) resolveShareTarget(ctx context.Context, share *entity.ShareLink) (*shareTargetResolution, error) {
	node, err := s.resolveShareTargetNode(ctx, share)
	if err != nil {
		return nil, err
	}
	if node != nil {
		return s.shareTargetFromNode(ctx, node)
	}
	return s.resolveLegacyShareTarget(ctx, share)
}

func (s *ShareService) resolveShareTargetNode(ctx context.Context, share *entity.ShareLink) (*entity.VFSNode, error) {
	if s.metadataReader == nil {
		return nil, nil
	}
	if share.TargetVFSNodeID > 0 {
		node, err := s.metadataReader.ResolveNodeByID(ctx, share.TargetVFSNodeID)
		if err != nil {
			return nil, ErrFileNotFound
		}
		if metadataVFSNodeUnavailable(node) {
			return nil, ErrFileNotFound
		}
		return node, nil
	}

	targetVirtualPath := share.TargetVirtualPath
	if strings.TrimSpace(targetVirtualPath) == "" {
		source, err := s.shareTargetSourceByID(ctx, share.SourceID)
		if err != nil {
			return nil, err
		}
		targetVirtualPath = mergeMountAndInnerPath(source.MountPath, share.Path)
	}
	if strings.TrimSpace(targetVirtualPath) == "" {
		return nil, ErrPathInvalid
	}
	normalizedPath, err := normalizeVirtualPath(targetVirtualPath)
	if err != nil {
		return nil, err
	}
	if err := s.refreshShareTargetMetadata(ctx, normalizedPath); err != nil {
		return nil, err
	}
	node, err := s.metadataReader.ResolveNode(ctx, normalizedPath)
	if err != nil {
		return nil, nil
	}
	if metadataVFSNodeUnavailable(node) {
		return nil, ErrFileNotFound
	}
	return node, nil
}

func (s *ShareService) shareTargetFromNode(ctx context.Context, node *entity.VFSNode) (*shareTargetResolution, error) {
	if node == nil || node.SourceID == nil || metadataVFSNodeUnavailable(node) {
		return nil, ErrFileNotFound
	}
	source, err := s.shareTargetSourceByID(ctx, *node.SourceID)
	if err != nil {
		return nil, err
	}
	innerPath, err := shareInnerPathForSource(source, node.Path)
	if err != nil {
		return nil, err
	}
	name := node.Name
	if name == "" && node.Path != "/" {
		name = path.Base(node.Path)
	}
	return &shareTargetResolution{
		Node:        node,
		Source:      source,
		VirtualPath: node.Path,
		InnerPath:   innerPath,
		Name:        name,
		IsDir:       metadataVFSNodeIsDirectory(node),
		NodeFirst:   true,
	}, nil
}

func (s *ShareService) resolveLegacyShareTarget(ctx context.Context, share *entity.ShareLink) (*shareTargetResolution, error) {
	sourceID := share.ResolvedSourceID
	if sourceID == 0 {
		sourceID = share.SourceID
	}
	source, err := s.shareTargetSourceByID(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	innerPath := share.ResolvedInnerPath
	if strings.TrimSpace(innerPath) == "" {
		innerPath = share.Path
	}
	virtualPath, name, isDir, err := s.inspectTarget(ctx, source, innerPath)
	if err != nil {
		return nil, err
	}
	targetVirtualPath := share.TargetVirtualPath
	if strings.TrimSpace(targetVirtualPath) == "" {
		targetVirtualPath = mergeMountAndInnerPath(source.MountPath, virtualPath)
	}
	if strings.TrimSpace(targetVirtualPath) == "" {
		targetVirtualPath = virtualPath
	}
	return &shareTargetResolution{
		Source:      source,
		VirtualPath: targetVirtualPath,
		InnerPath:   virtualPath,
		Name:        name,
		IsDir:       isDir,
		NodeFirst:   false,
	}, nil
}

func (s *ShareService) resolveSharePathTarget(ctx context.Context, root *shareTargetResolution, targetVirtualPath string) (*shareTargetResolution, error) {
	if root == nil || root.Source == nil {
		return nil, ErrFileNotFound
	}
	normalizedPath, err := normalizeVirtualPath(targetVirtualPath)
	if err != nil {
		return nil, err
	}
	if !isWithinShareRoot(root.VirtualPath, normalizedPath) {
		return nil, ErrPathInvalid
	}
	if root.NodeFirst && s.metadataReader != nil {
		_ = s.refreshShareMetadataPath(ctx, parentVirtualPath(normalizedPath))
		node, err := s.metadataReader.ResolveNode(ctx, normalizedPath)
		if err != nil {
			return nil, ErrFileNotFound
		}
		if metadataVFSNodeUnavailable(node) {
			return nil, ErrFileNotFound
		}
		return s.shareTargetFromNode(ctx, node)
	}

	innerPath, err := shareInnerPathForSource(root.Source, normalizedPath)
	if err != nil {
		return nil, err
	}
	virtualPath, name, isDir, err := s.inspectTarget(ctx, root.Source, innerPath)
	if err != nil {
		return nil, err
	}
	return &shareTargetResolution{
		Source:      root.Source,
		VirtualPath: normalizedPath,
		InnerPath:   virtualPath,
		Name:        name,
		IsDir:       isDir,
		NodeFirst:   false,
	}, nil
}

func (s *ShareService) shareTargetSourceByID(ctx context.Context, sourceID uint) (*entity.StorageSource, error) {
	source, err := s.sourceRepo.FindByID(ctx, sourceID)
	if err != nil {
		if errors.Is(err, domainrepo.ErrNotFound) {
			return nil, ErrFileNotFound
		}
		return nil, err
	}
	return source, nil
}

func (s *ShareService) inspectTarget(ctx context.Context, source *entity.StorageSource, pathValue string) (string, string, bool, error) {
	virtualPath, err := normalizeVirtualPath(pathValue)
	if err != nil {
		return "", "", false, err
	}

	if source.DriverType != "local" {
		driver, err := s.getFileDriver(source.DriverType)
		if err != nil {
			return "", "", false, err
		}
		entry, err := driver.Stat(ctx, source, virtualPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return "", "", false, ErrFileNotFound
			}
			return "", "", false, err
		}
		return virtualPath, entry.Name, entry.IsDir, nil
	}

	_, physicalPath, err := resolvePhysicalPath(source, virtualPath)
	if err != nil {
		return "", "", false, err
	}
	info, err := os.Stat(physicalPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", false, ErrFileNotFound
		}
		return "", "", false, err
	}
	return virtualPath, info.Name(), info.IsDir(), nil
}

func (s *ShareService) resolveShareTargetVFSNodeID(ctx context.Context, targetVirtualPath string) (uint, error) {
	if s.metadataReader == nil {
		return 0, nil
	}
	normalizedPath, err := normalizeVirtualPath(targetVirtualPath)
	if err != nil {
		return 0, err
	}
	refreshErr := s.refreshShareTargetMetadata(ctx, normalizedPath)
	node, err := s.metadataReader.ResolveNode(ctx, normalizedPath)
	if err == nil {
		return node.ID, nil
	}
	if refreshErr != nil {
		return 0, fmt.Errorf("%w: %w", ErrMetadataVFSCommitFailed, refreshErr)
	}
	return 0, fmt.Errorf("%w: %w", ErrMetadataVFSCommitFailed, err)
}

func (s *ShareService) refreshShareTargetMetadata(ctx context.Context, targetVirtualPath string) error {
	if s.metadataRefresh == nil {
		return nil
	}
	segments := strings.Split(strings.Trim(strings.TrimSpace(targetVirtualPath), "/"), "/")
	if len(segments) <= 1 {
		return s.refreshShareMetadataPath(ctx, "/")
	}
	current := "/"
	for index := 0; index < len(segments)-1; index++ {
		current = joinVirtualPath(current, segments[index])
		if err := s.refreshShareMetadataPath(ctx, current); err != nil {
			return err
		}
	}
	return nil
}

func (s *ShareService) refreshShareMetadataPath(ctx context.Context, virtualPath string) error {
	if s.metadataReader == nil || s.metadataRefresh == nil {
		return nil
	}
	node, err := s.metadataReader.ResolveNode(ctx, virtualPath)
	if err != nil {
		return nil
	}
	if node.SourceID == nil {
		return nil
	}
	if _, err := s.metadataRefresh.RefreshPath(ctx, node.Path); err != nil && !errors.Is(err, ErrVFSSyncConflict) {
		return err
	}
	return nil
}

func (s *ShareService) listPublicDirectory(ctx context.Context, root *shareTargetResolution, current *shareTargetResolution, sortBy, sortOrder string, page, pageSize int) ([]appdto.PublicShareEntry, int, int, error) {
	if root == nil || current == nil || current.Source == nil {
		return nil, 0, 0, ErrFileNotFound
	}
	if current.NodeFirst && s.metadataReader != nil {
		_ = s.refreshShareMetadataPath(ctx, current.VirtualPath)
		resp, err := s.metadataReader.ListChildren(ctx, current.VirtualPath)
		if err != nil {
			return nil, 0, 0, err
		}
		entriesView := toPublicShareEntriesFromVFSItems(resp.Items, root.VirtualPath)
		sortPublicShareEntries(entriesView, sortBy, sortOrder)
		pageItems, total, totalPages := paginateItems(entriesView, page, pageSize)
		return pageItems, total, totalPages, nil
	}

	source := current.Source
	actualPath := current.InnerPath
	if source.DriverType != "local" {
		driver, err := s.getFileDriver(source.DriverType)
		if err != nil {
			return nil, 0, 0, err
		}
		entries, err := driver.List(ctx, source, actualPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, 0, 0, ErrFileNotFound
			}
			return nil, 0, 0, err
		}
		items := make([]appdto.FileItem, 0, len(entries))
		for _, entry := range entries {
			if isHiddenStorageEntry(entry) {
				continue
			}
			items = append(items, buildStorageEntryItem(source.ID, entry))
		}
		sortFileItems(items, sortBy, sortOrder)
		entriesView := toPublicShareEntries(items, root.InnerPath)
		pageItems, total, totalPages := paginateItems(entriesView, page, pageSize)
		return pageItems, total, totalPages, nil
	}

	_, physicalPath, err := resolvePhysicalPath(source, actualPath)
	if err != nil {
		return nil, 0, 0, err
	}
	entries, err := os.ReadDir(physicalPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, 0, ErrFileNotFound
		}
		return nil, 0, 0, err
	}

	items := make([]appdto.FileItem, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".trash") || strings.HasPrefix(entry.Name(), ".system") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil, 0, 0, infoErr
		}
		itemPath := path.Join(actualPath, entry.Name())
		if actualPath == "/" {
			itemPath = "/" + entry.Name()
		}
		items = append(items, buildFileItem(source.ID, itemPath, info))
	}
	sortFileItems(items, sortBy, sortOrder)
	entriesView := toPublicShareEntries(items, root.InnerPath)
	pageItems, total, totalPages := paginateItems(entriesView, page, pageSize)
	return pageItems, total, totalPages, nil
}

func (s *ShareService) getFileDriver(driverType string) (FileDriver, error) {
	driver, exists := s.fileDrivers[driverType]
	if !exists {
		return nil, ErrSourceDriverUnsupported
	}
	return driver, nil
}

func (s *ShareService) currentUserID(ctx context.Context) (uint, error) {
	auth, ok := security.RequestAuthFromContext(ctx)
	if !ok {
		return 0, ErrPermissionDenied
	}
	return auth.UserID, nil
}

func (s *ShareService) authorizeSharePath(ctx context.Context, sourceID uint, pathValue string) error {
	if s.aclAuthorizer == nil {
		return nil
	}
	return s.aclAuthorizer.AuthorizePath(ctx, sourceID, pathValue, ACLActionShare)
}

func (s *ShareService) authorizeShareOwnership(ctx context.Context, share *entity.ShareLink, manage bool) error {
	auth, ok := security.RequestAuthFromContext(ctx)
	if !ok {
		return ErrPermissionDenied
	}
	allowed := permission.CanReadShare(auth.UserID, share.UserID, auth.Capabilities)
	if manage {
		allowed = permission.CanManageShare(auth.UserID, share.UserID, auth.Capabilities)
	}
	if !allowed {
		return ErrPermissionDenied
	}
	return nil
}

func (s *ShareService) ensureShareAccessible(share *entity.ShareLink, password string) error {
	if share.ExpiresAt != nil && s.now().After(*share.ExpiresAt) {
		return ErrShareExpired
	}
	if share.PasswordHash == nil {
		return nil
	}
	if password == "" {
		return ErrSharePasswordRequired
	}
	if !s.hasher.Compare(*share.PasswordHash, password) {
		return ErrSharePasswordInvalid
	}
	return nil
}

func toShareView(share *entity.ShareLink) appdto.ShareView {
	var expiresAt *string
	if share.ExpiresAt != nil {
		formatted := share.ExpiresAt.Format(time.RFC3339)
		expiresAt = &formatted
	}

	return appdto.ShareView{
		ID:                share.ID,
		SourceID:          share.SourceID,
		Path:              share.Path,
		TargetVFSNodeID:   share.TargetVFSNodeID,
		TargetVirtualPath: share.TargetVirtualPath,
		ResolvedSourceID:  share.ResolvedSourceID,
		ResolvedInnerPath: share.ResolvedInnerPath,
		Name:              share.Name,
		IsDir:             share.IsDir,
		Link:              path.Join("/s", share.Token),
		HasPassword:       share.PasswordHash != nil,
		ExpiresAt:         expiresAt,
		CreatedAt:         share.CreatedAt.Format(time.RFC3339),
	}
}

func toShareViewWithTarget(share *entity.ShareLink, target *shareTargetResolution) appdto.ShareView {
	view := toShareView(share)
	if target == nil || target.Source == nil {
		return view
	}
	view.SourceID = target.Source.ID
	view.Path = target.InnerPath
	view.TargetVFSNodeID = share.TargetVFSNodeID
	if target.Node != nil {
		view.TargetVFSNodeID = target.Node.ID
	}
	view.TargetVirtualPath = target.VirtualPath
	view.ResolvedSourceID = target.Source.ID
	view.ResolvedInnerPath = target.InnerPath
	if target.Name != "" {
		view.Name = target.Name
	}
	view.IsDir = target.IsDir
	return view
}

func (s *ShareService) buildShareDownloadURL(sourceID uint, filePath, virtualPath, disposition string, expiresAt *time.Time) (string, error) {
	if disposition == "" {
		disposition = "attachment"
	}
	tokenTTL := 5 * time.Minute
	if expiresAt != nil {
		remaining := expiresAt.Sub(s.now())
		if remaining <= 0 {
			return "", ErrShareExpired
		}
		if remaining < tokenTTL {
			tokenTTL = remaining
		}
	}

	normalizedFilePath, err := normalizeVirtualPath(filePath)
	if err != nil {
		return "", err
	}
	normalizedVirtualPath, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return "", err
	}

	fileToken, _, err := s.fileAccessTokens.Issue(sourceID, normalizedFilePath, "share", disposition, tokenTTL)
	if err != nil {
		return "", err
	}

	params := url.Values{}
	params.Set("path", normalizedVirtualPath)
	params.Set("disposition", disposition)
	params.Set("access_token", fileToken)
	return "/api/v2/fs/download?" + params.Encode(), nil
}

func resolveShareTargetPath(shareRootPath string, requestedPath string) (string, string, error) {
	rootPath, err := normalizeVirtualPath(shareRootPath)
	if err != nil {
		return "", "", err
	}
	relativePath, err := normalizeShareRelativePath(requestedPath)
	if err != nil {
		return "", "", err
	}
	if relativePath == "/" {
		return rootPath, relativePath, nil
	}

	actualPath := path.Join(rootPath, strings.TrimPrefix(relativePath, "/"))
	if rootPath == "/" {
		actualPath = relativePath
	}
	if !isWithinShareRoot(rootPath, actualPath) {
		return "", "", ErrPathInvalid
	}
	return actualPath, relativePath, nil
}

func normalizeShareRelativePath(input string) (string, error) {
	if input == "" {
		return "/", nil
	}
	if !strings.HasPrefix(input, "/") {
		return "", ErrPathInvalid
	}
	for _, segment := range strings.Split(strings.TrimPrefix(input, "/"), "/") {
		if segment == ".." {
			return "", ErrPathInvalid
		}
	}
	return normalizeVirtualPath(input)
}

func shareInnerPathForSource(source *entity.StorageSource, virtualPath string) (string, error) {
	if source == nil {
		return "", ErrFileNotFound
	}
	normalizedPath, err := normalizeVirtualPath(virtualPath)
	if err != nil {
		return "", err
	}
	mountPath, err := normalizeMountPath(source.MountPath)
	if err != nil {
		return "", err
	}
	if !isSubPath(mountPath, normalizedPath) {
		return "", ErrFileNotFound
	}
	if mountPath == "/" {
		return normalizedPath, nil
	}
	innerPath := strings.TrimPrefix(normalizedPath, mountPath)
	if innerPath == "" {
		return "/", nil
	}
	if !strings.HasPrefix(innerPath, "/") {
		innerPath = "/" + innerPath
	}
	return normalizeVirtualPath(innerPath)
}

func isWithinShareRoot(rootPath string, targetPath string) bool {
	if rootPath == "/" {
		return strings.HasPrefix(targetPath, "/")
	}
	if targetPath == rootPath {
		return true
	}
	return strings.HasPrefix(targetPath, strings.TrimSuffix(rootPath, "/")+"/")
}

func toPublicShareEntries(items []appdto.FileItem, shareRootPath string) []appdto.PublicShareEntry {
	entries := make([]appdto.PublicShareEntry, 0, len(items))
	for _, item := range items {
		if isHiddenPublicShareEntryName(item.Name) {
			continue
		}
		relativePath := publicShareRelativePath(shareRootPath, item.Path)
		entries = append(entries, appdto.PublicShareEntry{
			Name:         item.Name,
			Path:         relativePath,
			ParentPath:   publicShareParentPath(relativePath),
			IsDir:        item.IsDir,
			PreviewType:  publicSharePreviewType(item),
			Size:         item.Size,
			MimeType:     item.MimeType,
			Extension:    item.Extension,
			ModifiedAt:   item.ModifiedAt,
			CreatedAt:    item.CreatedAt,
			CanPreview:   item.CanPreview,
			CanDownload:  item.CanDownload,
			ThumbnailURL: item.ThumbnailURL,
		})
	}
	return entries
}

func toPublicShareEntriesFromVFSItems(items []appdto.VFSItem, shareRootPath string) []appdto.PublicShareEntry {
	entries := make([]appdto.PublicShareEntry, 0, len(items))
	for _, item := range items {
		if isHiddenPublicShareEntryName(item.Name) || isUnavailablePublicShareVFSItem(item) {
			continue
		}
		isDir := item.EntryKind == string(VirtualEntryKindDirectory)
		relativePath := publicShareRelativePath(shareRootPath, item.Path)
		thumbnail := (*string)(nil)
		entries = append(entries, appdto.PublicShareEntry{
			Name:         item.Name,
			Path:         relativePath,
			ParentPath:   publicShareParentPath(relativePath),
			IsDir:        isDir,
			PreviewType:  publicSharePreviewTypeFromParts(isDir, item.MimeType),
			Size:         item.Size,
			MimeType:     item.MimeType,
			Extension:    item.Extension,
			ModifiedAt:   item.ModifiedAt,
			CreatedAt:    item.CreatedAt,
			CanPreview:   item.CanPreview,
			CanDownload:  item.CanDownload,
			ThumbnailURL: thumbnail,
		})
	}
	return entries
}

func isUnavailablePublicShareVFSItem(item appdto.VFSItem) bool {
	switch item.SyncState {
	case entity.VFSNodeSyncStateMissing,
		entity.VFSNodeSyncStateError,
		entity.VFSNodeSyncStatePending,
		entity.VFSNodeSyncStateSyncing,
		entity.VFSNodeSyncStateConflict:
		return true
	default:
		return false
	}
}

func sortPublicShareEntries(entries []appdto.PublicShareEntry, sortBy, sortOrder string) {
	desc := strings.EqualFold(sortOrder, "desc")
	if sortBy == "" {
		sortBy = "name"
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		var cmp int
		switch sortBy {
		case "size":
			cmp = compareInt64(entries[i].Size, entries[j].Size)
		case "name":
			cmp = strings.Compare(strings.ToLower(entries[i].Name), strings.ToLower(entries[j].Name))
		default:
			cmp = strings.Compare(entries[i].ModifiedAt, entries[j].ModifiedAt)
		}
		if cmp == 0 {
			cmp = strings.Compare(strings.ToLower(entries[i].Name), strings.ToLower(entries[j].Name))
		}
		if cmp == 0 {
			cmp = strings.Compare(entries[i].Path, entries[j].Path)
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
}

func compareInt64(left int64, right int64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func isHiddenPublicShareEntryName(name string) bool {
	return strings.HasPrefix(name, ".trash") || strings.HasPrefix(name, ".system")
}

func publicSharePreviewType(item appdto.FileItem) string {
	if item.IsDir {
		return "directory"
	}
	return publicSharePreviewTypeFromParts(false, item.MimeType)
}

func publicSharePreviewTypeFromParts(isDir bool, mimeType string) string {
	if isDir {
		return "directory"
	}
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return "image"
	case strings.HasPrefix(mimeType, "video/"):
		return "video"
	case strings.HasPrefix(mimeType, "audio/"):
		return "audio"
	case mimeType == "application/pdf":
		return "pdf"
	case strings.HasSuffix(mimeType, "json"):
		return "json"
	case strings.HasPrefix(mimeType, "text/"):
		return "text"
	default:
		return "binary"
	}
}

func publicShareRelativePath(shareRootPath string, actualPath string) string {
	shareRootPath = strings.TrimSuffix(shareRootPath, "/")
	if shareRootPath == "" {
		shareRootPath = "/"
	}
	if shareRootPath == "/" {
		return actualPath
	}
	relative := strings.TrimPrefix(actualPath, shareRootPath)
	if relative == "" {
		return "/"
	}
	if !strings.HasPrefix(relative, "/") {
		return "/" + relative
	}
	return relative
}

func publicShareParentPath(relativePath string) string {
	parent := path.Dir(relativePath)
	if parent == "." {
		return "/"
	}
	return parent
}

func buildPublicShareCurrentDir(rootName string, currentPath string) appdto.PublicShareCurrentDir {
	if currentPath == "/" {
		return appdto.PublicShareCurrentDir{
			Name:       rootName,
			Path:       "/",
			ParentPath: "/",
			IsRoot:     true,
		}
	}

	return appdto.PublicShareCurrentDir{
		Name:       path.Base(currentPath),
		Path:       currentPath,
		ParentPath: publicShareParentPath(currentPath),
		IsRoot:     false,
	}
}

func buildPublicShareBreadcrumbs(rootName string, currentPath string) []appdto.PublicShareBreadcrumb {
	breadcrumbs := []appdto.PublicShareBreadcrumb{{
		Name: rootName,
		Path: "/",
	}}
	if currentPath == "/" {
		return breadcrumbs
	}

	current := ""
	segments := strings.Split(strings.TrimPrefix(currentPath, "/"), "/")
	for _, segment := range segments {
		if segment == "" {
			continue
		}
		current += "/" + segment
		breadcrumbs = append(breadcrumbs, appdto.PublicShareBreadcrumb{
			Name: segment,
			Path: current,
		})
	}
	return breadcrumbs
}
