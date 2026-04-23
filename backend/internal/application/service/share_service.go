package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

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
	fileDrivers map[string]FileDriver
	now         func() time.Time
}

// ShareOpenResult 表示公开分享访问结果。
type ShareOpenResult struct {
	RedirectURL string
	Data        *appdto.PublicShareOpenResponse
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
		return nil, err
	}
	if err := s.authorizeSharePath(ctx, source.ID, req.Path); err != nil {
		return nil, err
	}

	virtualPath, name, isDir, err := s.inspectTarget(ctx, source, req.Path)
	if err != nil {
		return nil, err
	}

	userID, err := s.currentUserID(ctx)
	if err != nil {
		return nil, err
	}

	var passwordHash *string
	if req.Password != "" {
		hashed, hashErr := s.hasher.Hash(req.Password)
		if hashErr != nil {
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

	share := &entity.ShareLink{
		UserID:            userID,
		SourceID:          source.ID,
		Path:              virtualPath,
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
		return nil, err
	}

	view := toShareView(share)
	return &view, nil
}

// Update 更新当前用户拥有的分享链接。
func (s *ShareService) Update(ctx context.Context, id uint, req appdto.UpdateShareRequest) (*appdto.ShareView, error) {
	share, err := s.shareRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeShareOwnership(ctx, share, true); err != nil {
		return nil, err
	}

	changed := false
	if req.Password != nil {
		if *req.Password == "" {
			share.PasswordHash = nil
		} else {
			hashed, hashErr := s.hasher.Hash(*req.Password)
			if hashErr != nil {
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
			return nil, err
		}
	}

	view := toShareView(share)
	return &view, nil
}

// Delete 删除当前用户拥有的分享链接。
func (s *ShareService) Delete(ctx context.Context, id uint) (*appdto.DeleteShareResponse, error) {
	share, err := s.shareRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeShareOwnership(ctx, share, true); err != nil {
		return nil, err
	}
	if err := s.shareRepo.Delete(ctx, id); err != nil {
		return nil, err
	}
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

	source, err := s.sourceRepo.FindByID(ctx, share.SourceID)
	if err != nil {
		return nil, err
	}

	if !share.IsDir {
		redirectURL, redirectErr := s.buildShareDownloadURL(source.ID, share.Path, disposition, share.ExpiresAt)
		if redirectErr != nil {
			return nil, redirectErr
		}
		return &ShareOpenResult{RedirectURL: redirectURL}, nil
	}

	actualPath, currentPath, err := resolveShareTargetPath(share.Path, relativePath)
	if err != nil {
		return nil, err
	}

	_, _, isDir, err := s.inspectTarget(ctx, source, actualPath)
	if err != nil {
		return nil, err
	}
	if !isDir {
		redirectURL, redirectErr := s.buildShareDownloadURL(source.ID, actualPath, disposition, share.ExpiresAt)
		if redirectErr != nil {
			return nil, redirectErr
		}
		return &ShareOpenResult{RedirectURL: redirectURL}, nil
	}

	items, total, totalPages, err := s.listPublicDirectory(ctx, source, share.Path, actualPath, sortBy, sortOrder, page, pageSize)
	if err != nil {
		return nil, err
	}

	return &ShareOpenResult{
		Data: &appdto.PublicShareOpenResponse{
			Share:       toShareView(share),
			CurrentPath: currentPath,
			CurrentDir:  buildPublicShareCurrentDir(share.Name, currentPath),
			Breadcrumbs: buildPublicShareBreadcrumbs(share.Name, currentPath),
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

func (s *ShareService) listPublicDirectory(ctx context.Context, source *entity.StorageSource, shareRootPath string, actualPath string, sortBy, sortOrder string, page, pageSize int) ([]appdto.PublicShareEntry, int, int, error) {
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
		entriesView := toPublicShareEntries(items, shareRootPath)
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
	entriesView := toPublicShareEntries(items, shareRootPath)
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

func (s *ShareService) buildShareDownloadURL(sourceID uint, filePath, disposition string, expiresAt *time.Time) (string, error) {
	if disposition == "" {
		disposition = "attachment"
	}
	tokenTTL := 5 * time.Minute
	if expiresAt != nil {
		remaining := time.Until(*expiresAt)
		if remaining <= 0 {
			return "", ErrShareExpired
		}
		if remaining < tokenTTL {
			tokenTTL = remaining
		}
	}

	fileToken, _, err := s.fileAccessTokens.Issue(sourceID, filePath, "share", disposition, tokenTTL)
	if err != nil {
		return "", err
	}

	params := url.Values{}
	params.Set("source_id", fmt.Sprintf("%d", sourceID))
	params.Set("path", filePath)
	params.Set("disposition", disposition)
	params.Set("access_token", fileToken)
	return "/api/v1/files/download?" + params.Encode(), nil
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

func publicSharePreviewType(item appdto.FileItem) string {
	if item.IsDir {
		return "directory"
	}
	switch {
	case strings.HasPrefix(item.MimeType, "image/"):
		return "image"
	case strings.HasPrefix(item.MimeType, "video/"):
		return "video"
	case strings.HasPrefix(item.MimeType, "audio/"):
		return "audio"
	case item.MimeType == "application/pdf":
		return "pdf"
	case strings.HasSuffix(item.MimeType, "json"):
		return "json"
	case strings.HasPrefix(item.MimeType, "text/"):
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
